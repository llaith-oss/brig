package ipfsutil

import (
	"io"

	log "github.com/Sirupsen/logrus"
	coreunix "github.com/ipfs/go-ipfs/core/coreunix"
	"github.com/jbenet/go-multihash"
)

type Reader interface {
	io.Reader
	io.Seeker
	io.Closer

	// TODO: ipfs supports this, we don't yet.
	// io.WriterTo
}

// Cat returns an io.Reader that reads from ipfs.
func Cat(node *Node, hash multihash.Multihash) (Reader, error) {
	reader, err := coreunix.Cat(node.Context, node.IpfsNode, hash.B58String())
	if err != nil {
		log.Warningf("ipfs cat: %v", err)
		return nil, err
	}

	return reader, nil
}