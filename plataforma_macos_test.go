//go:build darwin

package main

import (
	"encoding/xml"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// plistValido comproba que s é XML ben formado (un & ou < sen escapar rompería o plist
// e launchd ou o Finder ignorarían o ficheiro sen dicir nada).
func plistValido(t *testing.T, s string) {
	t.Helper()
	d := xml.NewDecoder(strings.NewReader(s))
	d.Strict = false // o DOCTYPE
	for {
		if _, err := d.Token(); err == io.EOF {
			return
		} else if err != nil {
			t.Fatalf("plist inválido: %v\n%s", err, s)
		}
	}
}

func TestPlistLaunchd(t *testing.T) {
	p := xerarPlistFB(rutasFileBrowser())
	plistValido(t, p)
	for _, quere := range []string{
		"<string>" + etiquetaFB + "</string>",
		"<string>/Library/Application Support/Piztu/FileBrowser/filebrowser</string>",
		"<string>/Library/Application Support/Piztu/FileBrowser/config.yaml</string>",
		"<key>UserName</key>\n\t<string>_www</string>",
		"<key>RunAtLoad</key>\n\t<true/>",
	} {
		if !strings.Contains(p, quere) {
			t.Errorf("o plist non leva %q:\n%s", quere, p)
		}
	}
}

func TestInfoPlistEscapa(t *testing.T) {
	p := xerarInfoPlist("Lúa & <cía>", "lua", "26.11.1")
	plistValido(t, p)
	if !strings.Contains(p, "<string>Lúa &amp; &lt;cía&gt;</string>") ||
		!strings.Contains(p, "<string>gal.omeu.piztu.lua</string>") {
		t.Errorf("Info.plist:\n%s", p)
	}
}

func TestRutasMacNonBorranSistema(t *testing.T) {
	// desinstalarFileBrowser fai RemoveAll de DirBin e desinstalarTao/Lua do bundle.
	if d := rutasFileBrowser().DirBin; !strings.HasPrefix(d, "/Library/Application Support/Piztu/") {
		t.Errorf("DirBin perigoso: %s", d)
	}
	for _, d := range []string{raizApp(dirTao()), raizApp(dirLua())} {
		if !strings.HasPrefix(d, "/Applications/") || !strings.HasSuffix(d, ".app") {
			t.Errorf("bundle perigoso: %s", d)
		}
	}
	if raizApp("/opt/x") != "/opt/x" {
		t.Error("raizApp cambiou unha ruta que non é dun bundle")
	}
}

func TestComiñasShell(t *testing.T) {
	if got := comiñasShell("/a b/it's"); got != `'/a b/it'\''s'` {
		t.Errorf("comiñasShell = %s", got)
	}
}

// plutil é o validador de plist do propio sistema: o que acepte el, acéptao launchd.
func TestPlistsPasanPlutil(t *testing.T) {
	dir := t.TempDir()
	for nome, contido := range map[string]string{
		"fb.plist":   xerarPlistFB(rutasFileBrowser()),
		"info.plist": xerarInfoPlist("Lúa", "lua", "26.11.1"),
	} {
		ruta := filepath.Join(dir, nome)
		os.WriteFile(ruta, []byte(contido), 0o644)
		if out, err := exec.Command("/usr/bin/plutil", "-lint", ruta).CombinedOutput(); err != nil {
			t.Errorf("%s: %v %s", nome, err, out)
		}
	}
}

func TestPrepararAppFaiUnBundle(t *testing.T) {
	app := filepath.Join(t.TempDir(), "Tao.app")
	exe := filepath.Join(app, "Contents", "MacOS", "tao")
	os.MkdirAll(filepath.Dir(exe), 0o755)
	os.WriteFile(exe, []byte("#!/bin/sh\n"), 0o755)
	if err := prepararApp("Tao", exe); err != nil {
		t.Fatal(err)
	}
	datos, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil || !strings.Contains(string(datos), "<string>tao</string>") {
		t.Fatalf("Info.plist: %v\n%s", err, datos)
	}
	if raizApp(filepath.Dir(exe)) != app {
		t.Errorf("raizApp = %s", raizApp(filepath.Dir(exe)))
	}
}
