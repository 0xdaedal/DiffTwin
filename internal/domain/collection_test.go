package domain

import "testing"

func TestCollectionTreeOperationsHandleNestedItemsAndMissingIDs(t *testing.T) {
	collection := Collection{
		Requests: []Request{{ID: "root-request", Name: "Root"}},
		Folders: []Folder{{
			ID: "parent",
			Folders: []Folder{{
				ID:       "child",
				Requests: []Request{{ID: "nested-request", Name: "Before"}},
			}},
		}},
	}

	if got := RequestByID(&collection, "nested-request"); got == nil || got.Name != "Before" {
		t.Fatalf("nested request lookup = %#v, want request", got)
	}
	if RequestByID(nil, "nested-request") != nil || RequestByID(&collection, "missing") != nil {
		t.Fatal("request lookup should return nil for nil collections and missing IDs")
	}
	if FolderByID(nil, "parent") != nil || FolderByID(&collection, "") != nil || FolderByID(&collection, "missing") != nil {
		t.Fatal("folder lookup should return nil for nil collections and missing IDs")
	}

	if !UpdateRequest(&collection, "nested-request", func(request *Request) {
		request.Name = "After"
	}) {
		t.Fatal("nested request update failed")
	}
	if got := RequestByID(&collection, "nested-request").Name; got != "After" {
		t.Fatalf("updated request name = %q, want After", got)
	}
	if UpdateRequest(&collection, "missing", func(*Request) {}) {
		t.Fatal("update unexpectedly succeeded for a missing request")
	}

	if !AppendRequestToFolder(&collection, "child", Request{ID: "added-request"}) {
		t.Fatal("request could not be appended to nested folder")
	}
	if AppendRequestToFolder(&collection, "missing", Request{ID: "invalid"}) {
		t.Fatal("request was appended to a missing folder")
	}
	if !AppendRequestToFolder(&collection, "", Request{ID: "second-root"}) {
		t.Fatal("request could not be appended to collection root")
	}
	if got := CountRequests(collection.Requests, collection.Folders); got != 4 {
		t.Fatalf("CountRequests() = %d, want 4", got)
	}

	if !RemoveRequest(&collection, "root-request") || !RemoveRequest(&collection, "nested-request") ||
		RemoveRequest(&collection, "missing") {
		t.Fatal("request removal did not handle root, nested, and missing IDs")
	}
	if !AppendFolder(&collection.Folders, "child", Folder{ID: "grandchild"}) {
		t.Fatal("folder could not be appended beneath a nested parent")
	}
	if AppendFolder(&collection.Folders, "missing", Folder{ID: "invalid"}) {
		t.Fatal("folder was appended beneath a missing parent")
	}
	if !AppendFolder(&collection.Folders, "", Folder{ID: "root-folder"}) {
		t.Fatal("folder could not be appended at collection root")
	}
	if !RemoveFolder(&collection.Folders, "grandchild") || !RemoveFolder(&collection.Folders, "parent") ||
		RemoveFolder(&collection.Folders, "missing") {
		t.Fatal("folder removal did not handle nested, root, and missing IDs")
	}
}
