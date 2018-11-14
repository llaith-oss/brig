package nodes

import (
	"testing"

	h "github.com/sahib/brig/util/hashlib"
	"github.com/stretchr/testify/require"
	capnp "zombiezen.com/go/capnproto2"
)

func TestCommit(t *testing.T) {
	cmt, err := NewEmptyCommit(0, 42)
	if err != nil {
		t.Fatalf("Failed to create commit: %v", err)
	}

	cmt.root = h.EmptyBackendHash
	cmt.parent = h.EmptyBackendHash
	cmt.Base.name = "some commit"

	cmt.SetMergeMarker(AuthorOfStage, h.TestDummy(t, 42))

	if err := cmt.BoxCommit(AuthorOfStage, "Hello"); err != nil {
		t.Fatalf("Failed to box commit: %v", err)
	}

	msg, err := cmt.ToCapnp()
	if err != nil {
		t.Fatalf("Failed to convert commit to capnp: %v", err)
	}

	data, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Failed to marshal message: %v", err)
	}

	newMsg, err := capnp.Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	empty := &Commit{}
	if err := empty.FromCapnp(newMsg); err != nil {
		t.Fatalf("From failed: %v", err)
	}

	if empty.message != "Hello" {
		t.Fatalf("Bad message unmarshaled: %v", empty.message)
	}

	if !empty.root.Equal(h.EmptyBackendHash) {
		t.Fatalf("Bad root unmarshaled: %v", empty.root)
	}

	if !empty.parent.Equal(h.EmptyBackendHash) {
		t.Fatalf("Bad parent unmarshaled: %v", empty.root)
	}

	if empty.author != AuthorOfStage {
		t.Fatalf("Bad author unmarshaled: %v", empty.root)
	}

	if empty.index != 42 {
		t.Fatalf("Index did not match in umarshalled: %v", empty.index)
	}

	person, remoteHead := empty.MergeMarker()
	if !remoteHead.Equal(h.TestDummy(t, 42)) {
		t.Fatalf("Remote head was not loaded correctly: %v", remoteHead.Bytes())
	}

	if person != AuthorOfStage {
		t.Fatalf("Person from unmarshaled commit does not equal staging author: %v", person)
	}

	empty.modTime = cmt.modTime
	require.Equal(t, empty, cmt)
}
