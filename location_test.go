package devtest_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"testing"

	"github.com/protolambda/devtest"
	"github.com/protolambda/mustbe/be"
	"github.com/protolambda/proto-log/log"
)

const locationChildEnv = "DEVTEST_LOCATION_CHILD"

// wantNextLine prints the file:line of the statement after the call,
// for the parent test to match against the location in the test output.
func wantNextLine() {
	fmt.Printf("WANT %s\n", lineAfter(1))
}

// lineAfter returns the file:line of the statement after the caller at the given depth.
// A skip of 0 is the caller of lineAfter.
func lineAfter(skip int) string {
	_, _, line, _ := runtime.Caller(skip + 1)
	return fmt.Sprintf("location_test.go:%d", line+1)
}

type panicAssertion struct{}

func (panicAssertion) String() string { return "panics" }

func (panicAssertion) Check(ctx context.Context) error {
	panic("reported")
}

func helperError(t devtest.T) {
	t.Helper()
	t.Error("reported")
}

func nestedHelperError(t devtest.T) {
	t.Helper()
	helperError(t)
}

func helperMust(t devtest.T) {
	t.Helper()
	t.Must(be.True(false))
}

func tbHelperError(tb testing.TB) {
	tb.Helper()
	tb.Error("reported")
}

func helperSubTest(t devtest.T) {
	t.Helper()
	t.Run("sub", func(t devtest.T) {
		wantNextLine()
		t.Error("reported")
	})
}

// locationCases produce output with a source location, expected at the line after wantNextLine.
var locationCases = map[string]func(t devtest.T){
	"Log": func(t devtest.T) {
		wantNextLine()
		t.Log("reported")
	},
	"Logf": func(t devtest.T) {
		wantNextLine()
		t.Logf("%s", "reported")
	},
	"Error": func(t devtest.T) {
		wantNextLine()
		t.Error("reported")
	},
	"Errorf": func(t devtest.T) {
		wantNextLine()
		t.Errorf("%s", "reported")
	},
	"Fatal": func(t devtest.T) {
		wantNextLine()
		t.Fatal("reported")
	},
	"Fatalf": func(t devtest.T) {
		wantNextLine()
		t.Fatalf("%s", "reported")
	},
	"Skip": func(t devtest.T) {
		wantNextLine()
		t.Skip("reported")
	},
	"Skipf": func(t devtest.T) {
		wantNextLine()
		t.Skipf("%s", "reported")
	},
	"Must": func(t devtest.T) {
		wantNextLine()
		t.Must(be.True(false))
	},
	"Mustf": func(t devtest.T) {
		wantNextLine()
		t.Mustf(be.True(false), "%s", "reported")
	},
	"MustPanic": func(t devtest.T) {
		wantNextLine()
		t.Must(panicAssertion{})
	},
	"Helper": func(t devtest.T) {
		wantNextLine()
		helperError(t)
	},
	"NestedHelper": func(t devtest.T) {
		wantNextLine()
		nestedHelperError(t)
	},
	"HelperMust": func(t devtest.T) {
		wantNextLine()
		helperMust(t)
	},
	"TBHelper": func(t devtest.T) {
		wantNextLine()
		tbHelperError(t)
	},
	"WithContextHelper": func(t devtest.T) {
		t = t.WithContext(t.Context())
		wantNextLine()
		helperError(t)
	},
	"SubTest": func(t devtest.T) {
		helperSubTest(t)
	},
	"Parallel": func(t devtest.T) {
		wantNextLine()
		t.Parallel()
	},
}

// TestLocationChild produces the output of the location cases.
// It only runs as subprocess of TestLocation.
func TestLocationChild(gt *testing.T) {
	if os.Getenv(locationChildEnv) != "1" {
		gt.Skip("only runs as subprocess of TestLocation")
	}
	for name, fn := range locationCases {
		gt.Run(name, func(gt *testing.T) {
			fn(devtest.SerialT(gt))
		})
	}
}

var (
	wantRe = regexp.MustCompile(`WANT (\S+:\d+)`)
	// Both the testing package and the test-logger start the line with the source location.
	gotRe = regexp.MustCompile(`(?m)^\s*(\S+\.go:\d+)\b`)
)

// TestLocation checks that the source location of the test output is the line of the test,
// not a line inside devtest, mustbe or test helpers, by inspecting the output of the real testing package.
func TestLocation(t *testing.T) {
	if os.Getenv(locationChildEnv) == "1" {
		t.Skip("parent test")
	}
	for name := range locationCases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command(os.Args[0],
				"-test.run=^TestLocationChild$/^"+regexp.QuoteMeta(name)+"$",
				"-test.v", "-test.count=1")
			cmd.Env = append(os.Environ(), locationChildEnv+"=1")
			out, _ := cmd.CombinedOutput()
			wantLoc := wantRe.FindSubmatchIndex(out)
			if wantLoc == nil {
				t.Fatalf("could not find wanted location in output:\n%s", out)
			}
			// The first location after the WANT line
			got := gotRe.FindSubmatch(out[wantLoc[1]:])
			if got == nil {
				t.Fatalf("could not find reported location in output:\n%s", out)
			}
			if want := out[wantLoc[2]:wantLoc[3]]; !bytes.Equal(want, got[1]) {
				t.Errorf("expected output at %s, got %s, output:\n%s", want, got[1], out)
			}
		})
	}
}

func helperPLog(p devtest.P) {
	p.Helper()
	p.Log("reported")
}

// TestLocationP checks the source location of the P output.
func TestLocationP(gt *testing.T) {
	t := devtest.SerialT(gt)
	wd, err := os.Getwd()
	t.Must(be.NoError(err))
	var buf bytes.Buffer
	logger := log.New(log.TerminalHandler(&buf,
		log.WithIncludeSource(true), log.WithSourceRelDir(wd)))

	var want []string
	out := devtest.Run(t.Context(), logger, func(p devtest.P) {
		want = append(want, lineAfter(0))
		p.Log("reported")
		want = append(want, lineAfter(0))
		p.Errorf("%s", "reported")
		want = append(want, lineAfter(0))
		helperPLog(p)
		want = append(want, lineAfter(0))
		helperPLog(p.WithContext(p.Context()))
		want = append(want, lineAfter(0))
		p.Must(be.True(false))
	})
	<-out.Done()

	got := gotRe.FindAllSubmatch(buf.Bytes(), -1)
	gotLocs := make([]string, 0, len(got))
	for _, m := range got {
		gotLocs = append(gotLocs, string(m[1]))
	}
	t.Mustf(be.DeepEqual(want, gotLocs), "output:\n%s", buf.String())
}
