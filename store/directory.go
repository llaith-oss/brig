package store

import (
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/disorganizer/brig/store/wire"
	"github.com/gogo/protobuf/proto"
	goipfsutil "github.com/ipfs/go-ipfs-util"
	"github.com/jbenet/go-multihash"
)

type Directory struct {
	name     string
	size     uint64
	modTime  time.Time
	parent   *Hash
	hash     *Hash
	children map[string]*Hash
	id       uint64

	// This is not set by FromProto() and must be passed
	// on creating by FS.
	fs *FS
}

// newEmptyDirectory creates a new empty directory that is not yet present
// in the store. It should not be used directtly.
func newEmptyDirectory(fs *FS, parent *Directory, name string) (*Directory, error) {
	code := goipfsutil.DefaultIpfsHash
	length := multihash.DefaultLengths[code]

	path := path.Join(NodePath(parent), name)
	mh, err := multihash.Sum([]byte(path), code, length)
	if err != nil {
		// The programmer has likely fucked up:
		return nil, fmt.Errorf("Failed to calculate basic checksum of a string: %v", err)
	}

	id, err := fs.NextID()
	if err != nil {
		return nil, err
	}

	dir := &Directory{
		fs:   fs,
		id:   id,
		hash: &Hash{mh},
		name: name,
	}

	if parent != nil {
		if err := parent.Add(dir); err != nil {
			return nil, err
		}
	}

	return dir, nil
}

////////////// MARSHALLING ////////////////

func (d *Directory) ToProto() (*wire.Node, error) {
	binModTime, err := d.modTime.MarshalBinary()
	if err != nil {
		return nil, err
	}

	binLinks := [][]byte{}
	binNames := []string{}

	for name, link := range d.children {
		binLinks = append(binLinks, link.Bytes())
		binNames = append(binNames, name)
	}

	return &wire.Node{
		ID:       proto.Uint64(d.id),
		Type:     wire.NodeType_DIRECTORY.Enum(),
		ModTime:  binModTime,
		NodeSize: proto.Uint64(d.size),
		Hash:     d.hash.Bytes(),
		Name:     proto.String(d.name),
		Parent:   d.parent.Bytes(),
		Directory: &wire.Directory{
			Links: binLinks,
			Names: binNames,
		},
	}, nil
}

func (d *Directory) Marshal() ([]byte, error) {
	pbd, err := d.ToProto()
	if err != nil {
		return nil, err
	}

	return proto.Marshal(pbd)
}

func (d *Directory) FromProto(pnd *wire.Node) error {
	pbd := pnd.GetDirectory()

	modTime := time.Time{}
	if err := modTime.UnmarshalBinary(pnd.GetModTime()); err != nil {
		return err
	}

	d.id = pnd.GetID()
	d.modTime = modTime
	d.parent = &Hash{pnd.GetParent()}
	d.size = pnd.GetNodeSize()
	d.hash = &Hash{pnd.GetHash()}
	d.name = pnd.GetName()
	d.children = make(map[string]*Hash)

	// Find our place in the world:
	links := pbd.GetLinks()
	for idx, name := range pbd.GetNames() {
		// Be cautious, input might come from everywhere:
		if idx >= 0 && idx < len(links) {
			return fmt.Errorf("Malformed input: More or less names than links in `%s`", d.name)
		}

		d.children[name] = &Hash{links[idx]}
	}

	return nil
}

func (d *Directory) Unmarshal(data []byte) error {
	pbd := &wire.Node{}
	if err := proto.Unmarshal(data, pbd); err != nil {
		return err
	}

	return d.FromProto(pbd)
}

////////////// NODE INTERFACE /////////////////

func (d *Directory) Name() string {
	return d.name
}

func (d *Directory) Hash() *Hash {
	return d.hash
}

func (d *Directory) Size() uint64 {
	return d.size
}

func (d *Directory) ModTime() time.Time {
	return d.modTime
}

func (d *Directory) NChildren() int {
	return len(d.children)
}

func (d *Directory) Child(name string) (Node, error) {
	// TODO: efficient lookup?
	return nil, nil
}

func (d *Directory) Parent() (Node, error) {
	if d.parent == nil {
		return nil, nil
	}

	return d.fs.NodeByHash(d.parent)
}

func (d *Directory) SetParent(nd Node) error {
	if nd == nil {
		d.parent = EmptyHash
	} else {
		d.parent = nd.Hash()
	}

	// TODO: error needed?
	return nil
}

func (d *Directory) GetType() NodeType {
	return NodeTypeDirectory
}

func (d *Directory) ID() uint64 {
	return d.id
}

////////////// TREE MOVEMENT /////////////////

func (d *Directory) VisitChildren(fn func(*Directory) error) error {
	for _, hash := range d.children {
		child, err := d.fs.DirectoryByHash(hash)
		if err != nil {
			return err
		}

		if err := fn(child); err != nil {
			return err
		}
	}

	return nil
}

func (d *Directory) Up(visit func(par *Directory) error) error {
	var err error

	for curr := d; curr.parent != nil; {
		if err := visit(curr); err != nil {
			return err
		}

		curr, err = d.fs.DirectoryByHash(curr.parent)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *Directory) xorHash(hash *Hash) error {
	if err := d.hash.Xor(hash); err != nil {
		return err
	}

	// We need to update the direct children since the parent hash changed.
	return d.VisitChildren(func(child *Directory) error {
		return child.SetParent(d)
	})
}

func Walk(node Node, dfs bool, visit func(child Node) error) error {
	if node.GetType() != NodeTypeDirectory {
		return visit(node)
	}

	d, ok := node.(*Directory)
	if !ok {
		return ErrBadNode
	}

	if !dfs {
		if err := visit(node); err != nil {
			return err
		}
	}

	for _, link := range d.children {
		child, err := d.fs.NodeByHash(link)
		if err != nil {
			return err
		}

		return Walk(child, dfs, visit)
	}

	if dfs {
		if err := visit(node); err != nil {
			return err
		}
	}

	return nil
}

func (d *Directory) Lookup(repoPath string) (Node, error) {
	repoPath = prefixSlash(path.Clean(repoPath))
	elems := strings.Split(repoPath, "/")

	if len(elems) == 1 {
		return d, nil
	}

	var curr Node = d
	var err error

	for _, elem := range elems {
		curr, err = curr.Child(elem)
		if err != nil {
			return nil, err
		}

		if curr == nil {
			return nil, nil
		}
	}

	return curr, nil
}

//////////// STATE ALTERING METHODS //////////////

// TODO: Grafik dafür in der Masterarbeit machen!
func (d *Directory) Add(nd Node) error {
	d.children[nd.Name()] = nd.Hash()
	nodeSize := nd.Size()
	nodeHash := nd.Hash()

	return d.Up(func(parent *Directory) error {
		parent.size += nodeSize
		return parent.xorHash(nodeHash)
	})
}

// RemoveChild removes the child named `name` from it's children.
//
// Note that there is no general Remove() function that works on itself.
// It is therefore not possible (or a good idea) to remove the root node.
func (d *Directory) RemoveChild(nd Node) error {
	name := nd.Name()
	if _, ok := d.children[name]; !ok {
		return NoSuchFile(name)
	}

	// Unset parent from child:
	if err := nd.SetParent(nil); err != nil {
		return err
	}

	delete(d.children, name)

	nodeSize := nd.Size()
	nodeHash := nd.Hash()

	return d.Up(func(parent *Directory) error {
		parent.size -= nodeSize
		return parent.xorHash(nodeHash)
	})
}
