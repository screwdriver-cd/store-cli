package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
)

func fakeExecCommand(command string, args ...string) *exec.Cmd {
	cs := []string{"-test.run=TestHelperProcess", "--", command}
	cs = append(cs, args...)
	cmd := exec.Command(os.Args[0], cs...)
	cmd.Env = []string{"GO_WANT_HELPER_PROCESS=1"}
	return cmd
}

const habExecResult = "run hab pkg install\nrun hab pkg exec\n"

func TestRunCommand(t *testing.T) {
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()

	stdout := new(bytes.Buffer)
	err := runCommand("sudo hab pkg install foo/bar", stdout)
	expected := "run hab pkg install\n"
	if err != nil {
		t.Errorf("runCommand error = %q, should be nil", err)
	}
	if string(stdout.Bytes()) != expected {
		t.Errorf("Expected '%v', actual '%v'", expected, string(stdout.Bytes()))
	}

	stdout = new(bytes.Buffer)
	err = runCommand("hab pkg exec foo/bar foo bar foobar", stdout)
	expected = "run hab pkg exec\n"
	if err != nil {
		t.Errorf("runCommand error = %v, should be nil", err)
	}
	if string(stdout.Bytes()) != expected {
		t.Errorf("Expected '%v', actual '%v'", expected, string(stdout.Bytes()))
	}
}

func TestExecHab(t *testing.T) {
	stdout := new(bytes.Buffer)
	execCommand = fakeExecCommand
	defer func() { execCommand = exec.Command }()
	err := execHab("foo/bar", "2.2.2", "stable", []string{"foo", "bar", "foobar"}, stdout)
	if err != nil {
		t.Errorf("execHab error = %q, should be nil", err)
	}
	if string(stdout.Bytes()) != habExecResult {
		t.Errorf("Expected %q, got %q", habExecResult, string(stdout.Bytes()))
	}
}

func TestMain(m *testing.M) {
	retCode := m.Run()
	os.Exit(retCode)
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	defer os.Exit(0)
	args := os.Args[:]
	for i, val := range os.Args {
		args = os.Args[i:]
		if val == "-c" {
			args = strings.Split(args[1:][0], " ")
			break
		}
	}

	if len(args) >= 4 {
		if args[0] == "sudo" && args[3] == "install" ||
			args[0] != "sudo" && args[2] == "install" {
			fmt.Println("run hab pkg install")
			return
		} else if args[2] == "exec" {
			fmt.Println("run hab pkg exec")
			return
		} else {
			os.Exit(255)
		}
	}
	os.Exit(255)
}

type depotMock struct {
	versions []string
	err      error
}

func (depo *depotMock) PackageVersionsFromName(pkgName string, habChannel string) ([]string, error) {
	if depo.err != nil {
		return nil, depo.err
	}
	return depo.versions, nil
}

func TestGetPackageVersions(t *testing.T) {
	tests := []struct {
		versionExpression string
		foundVersions     []string
		expectedVersion   string
		depotError        error
		expectedError     error
	}{
		{
			versionExpression: "1.0.0",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.0.0", "2.0.0"},
			expectedVersion:   "1.0.0",
			depotError:        nil,
			expectedError:     nil,
		},
		{
			versionExpression: "^1.2.0",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.3.0", "2.0.0"},
			expectedVersion:   "1.3.0",
			depotError:        nil,
			expectedError:     nil,
		},
		{
			versionExpression: "~1.2.0",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.3.0", "2.0.0"},
			expectedVersion:   "1.2.2",
			depotError:        nil,
			expectedError:     nil,
		},
		{
			versionExpression: "",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.3.0", "2.0.0"},
			expectedVersion:   "",
			depotError:        nil,
			expectedError:     nil,
		},
		{
			versionExpression: "1.0.0",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.3.0", "2.0.0"},
			expectedVersion:   "",
			depotError:        errors.New("depot error"),
			expectedError:     errors.New("The specified version not found"),
		},
		{
			versionExpression: "~1.2.0",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.2.3-abc", "1.3.0", "2.0.0"},
			expectedVersion:   "1.2.2",
			depotError:        nil,
			expectedError:     nil,
		},
		{
			versionExpression: "1.2.0-beta",
			foundVersions:     []string{"0.0.1", "0.1.0", "1.1.9", "1.2.1", "1.2.2", "1.2.3-abc", "1.3.0", "2.0.0"},
			expectedVersion:   "",
			depotError:        nil,
			expectedError:     errors.New("The specified version not found"),
		},
	}

	for _, test := range tests {
		depot := &depotMock{test.foundVersions, test.depotError}
		version, err := getPackageVersion(depot, "foo/test", test.versionExpression, "stable")

		if test.expectedError == nil && err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if test.expectedError != nil {
			if reflect.TypeOf(err) != reflect.TypeOf(test.expectedError) {
				t.Fatalf("Expected error type %v, actual %v", reflect.TypeOf(test.expectedError), reflect.TypeOf(err))
			} else if err.Error() != test.expectedError.Error() {
				t.Errorf("Expected Error \"%s\", actual \"%s\"", test.expectedError.Error(), err.Error())
			}
		} else {
			if version != test.expectedVersion {
				t.Errorf("Expected \"%s\", actual \"%s\"", test.expectedVersion, version)
			}
		}
	}
}
