//go:build darwin

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// stubDir creates a temporary directory with stub executables for every tool
// the packaging script invokes. Each stub appends its argv to $CALL_LOG and
// exits 0. Returns the dir path.
func stubDir(t *testing.T, callLog string) string {
	t.Helper()
	dir := t.TempDir()

	// go stub: parse -o flag and create a dummy binary so the script's
	// subsequent mv/chmod succeed.
	goStub := `#!/bin/sh
echo "$0 $*" >> "` + callLog + `"
# Parse -o <path> and create a dummy binary.
i=1; while [ $i -le $# ]; do
  eval "a=\${$i}"
  if [ "$a" = "-o" ]; then
    i=$((i+1)); eval "out=\${$i}"
    printf '#!/bin/sh\necho dummy\n' > "$out" && chmod +x "$out"
    exit 0
  fi
  i=$((i+1))
done
`
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(goStub), 0o755); err != nil {
		t.Fatal(err)
	}

	// mktemp stub: must actually create a temp dir and print the path.
	mktempStub := `#!/bin/sh
echo "$0 $*" >> "` + callLog + `"
/usr/bin/mktemp "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "mktemp"), []byte(mktempStub), 0o755); err != nil {
		t.Fatal(err)
	}

	// du stub: delegate to real du so the size line works.
	duStub := `#!/bin/sh
echo "$0 $*" >> "` + callLog + `"
/usr/bin/du "$@"
`
	if err := os.WriteFile(filepath.Join(dir, "du"), []byte(duStub), 0o755); err != nil {
		t.Fatal(err)
	}

	// All other stubs just log and exit 0.
	others := []string{"sips", "iconutil", "plutil", "codesign", "xcrun", "ditto", "hdiutil"}
	for _, name := range others {
		p := filepath.Join(dir, name)
		body := "#!/bin/sh\necho \"$0 $*\" >> \"" + callLog + "\"\n"
		if err := os.WriteFile(p, []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// fakeProject sets up the minimal directory structure the packaging script
// expects: webapp/dist/ (non-empty), desktop/packaging/Info.plist, and the
// script itself. Returns the project root.
func fakeProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// webapp/dist must be non-empty.
	if err := os.MkdirAll(filepath.Join(root, "webapp/dist"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "webapp/dist/index.html"), []byte("<html/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	// desktop/packaging/Info.plist + appicon.png.
	pkgDir := filepath.Join(root, "desktop/packaging")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		t.Fatal(err)
	}
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict><key>CFBundleName</key><string>DeepSeekCode</string></dict></plist>`
	if err := os.WriteFile(filepath.Join(pkgDir, "Info.plist"), []byte(plist), 0o644); err != nil {
		t.Fatal(err)
	}
	// appicon.png — minimal 1x1 PNG.
	png := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, 0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, 0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc, 0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e,
		0x44, 0xae, 0x42, 0x60, 0x82}
	if err := os.WriteFile(filepath.Join(pkgDir, "appicon.png"), png, 0o644); err != nil {
		t.Fatal(err)
	}

	// Copy the real packaging script (lives in desktop/, one level up).
	// Replace absolute paths (/usr/bin/X) with bare command names so the
	// stub executables on PATH are found instead of the real system tools.
	realScript, err := filepath.Abs(filepath.Join("..", "package-darwin.sh"))
	if err != nil {
		t.Fatal(err)
	}
	scriptData, err := os.ReadFile(realScript)
	if err != nil {
		t.Fatal(err)
	}
	s := string(scriptData)
	for _, tool := range []string{"codesign", "sips", "iconutil", "plutil", "ditto", "hdiutil"} {
		s = strings.ReplaceAll(s, "/usr/bin/"+tool, tool)
	}
	if err := os.WriteFile(filepath.Join(root, "desktop/package-darwin.sh"), []byte(s), 0o755); err != nil {
		t.Fatal(err)
	}

	// Copy entitlements.plist (same directory as this test file).
	entSrc, err := filepath.Abs("entitlements.plist")
	if err != nil {
		t.Fatal(err)
	}
	entData, err := os.ReadFile(entSrc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "entitlements.plist"), entData, 0o644); err != nil {
		t.Fatal(err)
	}

	return root
}

func TestPackagerSignsWhenIdentitySet(t *testing.T) {
	root := fakeProject(t)
	callLog := filepath.Join(root, "calls.log")
	stubs := stubDir(t, callLog)

	cmd := exec.Command("bash", filepath.Join(root, "desktop/package-darwin.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+stubs+":"+os.Getenv("PATH"),
		"HOME="+root,
		"DSC_SIGN_IDENTITY=Developer ID Application: Test",
		"DSC_NOTARY_PROFILE=dsc-notary",
		"DSC_MAKE_DMG=1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("packager failed: %v\n%s", err, out)
	}

	log, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("reading call log: %v", err)
	}
	s := string(log)

	// Developer ID signing must use --options runtime.
	if !strings.Contains(s, "--options runtime") {
		t.Errorf("call log missing --options runtime:\n%s", s)
	}
	// Notary submission.
	if !strings.Contains(s, "notarytool submit") {
		t.Errorf("call log missing notarytool submit:\n%s", s)
	}
	// Stapling.
	if !strings.Contains(s, "stapler staple") {
		t.Errorf("call log missing stapler staple:\n%s", s)
	}
	// DMG creation.
	if !strings.Contains(s, "hdiutil create") {
		t.Errorf("call log missing hdiutil create:\n%s", s)
	}
}

func TestPackagerFallsBackToAdHoc(t *testing.T) {
	root := fakeProject(t)
	callLog := filepath.Join(root, "calls.log")
	stubs := stubDir(t, callLog)

	cmd := exec.Command("bash", filepath.Join(root, "desktop/package-darwin.sh"))
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"PATH="+stubs+":"+os.Getenv("PATH"),
		"HOME="+root,
		// No DSC_SIGN_IDENTITY — should fall back to ad-hoc.
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("packager failed: %v\n%s", err, out)
	}

	log, err := os.ReadFile(callLog)
	if err != nil {
		t.Fatalf("reading call log: %v", err)
	}
	s := string(log)

	// Ad-hoc signing must use --sign - (dash).
	if !strings.Contains(s, "--sign -") {
		t.Errorf("call log missing ad-hoc --sign -:\n%s", s)
	}
	// Must NOT call notarytool.
	if strings.Contains(s, "notarytool") {
		t.Errorf("call log should not contain notarytool in ad-hoc mode:\n%s", s)
	}
	// Must NOT call stapler.
	if strings.Contains(s, "stapler") {
		t.Errorf("call log should not contain stapler in ad-hoc mode:\n%s", s)
	}
	// Must NOT create DMG (DSC_MAKE_DMG not set).
	if strings.Contains(s, "hdiutil") {
		t.Errorf("call log should not contain hdiutil when DSC_MAKE_DMG is unset:\n%s", s)
	}
}
