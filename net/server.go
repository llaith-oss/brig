package net

import (
	"context"
	"fmt"
	"net"

	"zombiezen.com/go/capnproto2/rpc"

	log "github.com/Sirupsen/logrus"
	"github.com/sahib/brig/backend"
	"github.com/sahib/brig/net/capnp"
	"github.com/sahib/brig/net/peer"
	"github.com/sahib/brig/repo"
	"github.com/sahib/brig/util/server"
)

type Server struct {
	bk         backend.Backend
	baseServer *server.Server
	hdl        *handler
	pingMap    *PingMap
}

func (sv *Server) Serve() error {
	return sv.baseServer.Serve()
}

func (sv *Server) Close() error {
	return sv.baseServer.Close()
}

func (sv *Server) Quit() {
	sv.baseServer.Quit()
}

func publishSelf(bk backend.Backend, owner string) error {
	// Example: alice@wonderland.org/resource
	name := peer.Name(owner)

	// Publish the full name.
	if err := bk.PublishName(owner); err != nil {
		return err
	}

	// Also publish alice@wonderland.org
	if noRes := name.WithoutResource(); noRes != string(name) {
		if err := bk.PublishName(noRes); err != nil {
			return err
		}
	}

	// Publish wonderland.org
	if domain := name.Domain(); domain != "" {
		if err := bk.PublishName(domain); err != nil {
			return err
		}
	}

	if user := name.User(); user != string(name) {
		if err := bk.PublishName(user); err != nil {
			return err
		}
	}

	return nil
}

func NewServer(rp *repo.Repository, bk backend.Backend) (*Server, error) {
	hdl := &handler{
		rp: rp,
		bk: bk,
	}

	lst, err := bk.Listen("brig/caprpc")
	if err != nil {
		return nil, err
	}

	ctx := context.Background()
	baseServer, err := server.NewServer(lst, hdl, ctx)
	if err != nil {
		return nil, err
	}

	if err := publishSelf(bk, rp.Owner); err != nil {
		log.Warningf("Failed to publish `%v` to the network: %v", rp.Owner, err)
		log.Warningf("You will not be visible to other users.")
	}

	return &Server{
		baseServer: baseServer,
		bk:         bk,
		hdl:        hdl,
		pingMap:    NewPingMap(bk),
	}, nil
}

func (sv *Server) Locate(who peer.Name) ([]peer.Info, error) {
	// TODO: Provide more locate options here. (domain, user etc.)
	return sv.bk.ResolveName(who.WithoutResource())
}

func (sv *Server) Identity() (peer.Info, error) {
	return sv.bk.Identity()
}

func (sv *Server) PingMap() *PingMap {
	return sv.pingMap
}

func (sv *Server) IsOnline() bool {
	return sv.bk.IsOnline()
}

func (sv *Server) Connect() error {
	return sv.bk.Connect()
}

func (sv *Server) Disconnect() error {
	return sv.bk.Disconnect()
}

/////////////////////////////////////
// INTERNAL HANDLER IMPLEMENTATION //
/////////////////////////////////////

type handler struct {
	bk backend.Backend
	rp *repo.Repository
}

func (hdl *handler) Handle(ctx context.Context, conn net.Conn) {
	keyring := hdl.rp.Keyring()
	ownPubKey, err := keyring.OwnPubKey()
	if err != nil {
		log.Warnf("Failed to retrieve own pubkey: %v", err)
		return
	}

	// Take the raw connection we get and add an authentication layer on top of it.
	authConn := NewAuthReadWriter(conn, keyring, ownPubKey, func(pubKey []byte) error {
		remotes, err := hdl.rp.Remotes.ListRemotes()
		if err != nil {
			return err
		}

		// Create a temporary fingerprint to get a hashed version of pubkey.
		remoteFp := peer.BuildFingerprint("", pubKey)

		// Linear scan over all remotes.
		// If this proves to be a performance problem, we can fix it later.
		for _, remote := range remotes {
			if remote.Fingerprint.PubKeyID() == remoteFp.PubKeyID() {
				log.Infof("Starting connection with %s", remote.Fingerprint.Addr())
				return nil
			}
		}

		return fmt.Errorf("Remote uses no public key known to us")
	})

	// Trigger the authentication.
	// (would trigger with the first read/writer elsewhise)
	if err := authConn.Trigger(); err != nil {
		log.Warnf("Failed to authenticate connection: %v", err)
		return
	}

	transport := rpc.StreamTransport(conn)
	srv := capnp.API_ServerToClient(hdl)
	rpcConn := rpc.NewConn(transport, rpc.MainInterface(srv.Client))

	if err := rpcConn.Wait(); err != nil {
		log.Warnf("Serving rpc failed: %v", err)
	}

	if err := rpcConn.Close(); err != nil {
		// Close seems to be complaining that the conn was
		// already closed, but be safe and expect this.
		if err != rpc.ErrConnClosed {
			log.Warnf("Failed to close rpc conn: %v", err)
		}
	}
}

// Quit is being called by the base server implementation
func (hdl *handler) Quit() error {
	return nil
}
