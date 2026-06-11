package transfer

import (
	"testing"

	pb "mdemg/api/transferpb"
)

// BACKUP-RESTORE-VERIFY-001 live-caught: a NULL path exported as "" and
// re-imported as a literal empty string violates memorynode_path_unique on
// the second observation node. Empty path/name must stay absent.
func TestNodeProps_OmitsEmptyPathAndName(t *testing.T) {
	nd := &pb.NodeData{
		SpaceId:  "s",
		NodeId:   "n1",
		RoleType: "conversation_observation",
	}
	props := nodeProps(nd)
	if _, ok := props["path"]; ok {
		t.Fatal("empty path must be omitted (constraint collision class)")
	}
	if _, ok := props["name"]; ok {
		t.Fatal("empty name must be omitted (null fidelity)")
	}
	if props["space_id"] != "s" || props["node_id"] != "n1" {
		t.Fatal("identity props missing")
	}
}

func TestNodeProps_KeepsRealPathAndName(t *testing.T) {
	nd := &pb.NodeData{SpaceId: "s", NodeId: "n2", Path: "internal/x.go", Name: "x"}
	props := nodeProps(nd)
	if props["path"] != "internal/x.go" || props["name"] != "x" {
		t.Fatalf("real path/name must survive: %v", props)
	}
}
