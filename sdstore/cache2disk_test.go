package sdstore

import (
	"testing"
)

// test Cache2Disk with invalid command
func TestCache2DiskInvalidCommand(t *testing.T) {
	if err := Cache2Disk("", "pipeline", ""); err != nil {
		t.Logf("Success, expected: `%v` ; got: `%v`", "Error: <nil>, command:  is not expected", err);
	} else {
		t.Errorf("FAILED, expected: `%v` ; got: `%v`", "Error: <nil>, command:  is not expected", err);
	}

}

// test Cache2Disk with invalid cache scope
func TestCache2DiskInvalidCacheScope(t *testing.T) {
	if err := Cache2Disk("", "", ""); err != nil {
		t.Logf("Success, expected: `%v` ; got: `%v`", "Error: <nil>, cache directory empty for cache scope  ", err);
	} else {
		t.Errorf("FAILED, expected: `%v` ; got: `%v`", "Error: <nil>, cache directory empty for cache scope  ", err);
	}

}
