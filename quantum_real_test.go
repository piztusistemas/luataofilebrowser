package main

// Proba contra un File Browser Quantum DE VERDADE o ciclo que fai instalarFileBrowser:
// primeiro arranque co contrasinal no config, login, quitar o contrasinal, reinicio e
// login de novo. Non instala servizo nin toca o firewall: lanza o binario nun cartafol
// temporal e nun porto libre. Só corre se se lle indica o binario:
//
//	FB_BIN=files/filebrowser.exe go test -run Quantum -v

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestQuantumRealCicloDoContrasinal(t *testing.T) {
	bin := os.Getenv("FB_BIN")
	if bin == "" {
		t.Skip("define FB_BIN co executable de File Browser Quantum")
	}
	bin, _ = filepath.Abs(bin)

	dir := t.TempDir()
	r := fbRutas{
		Config: filepath.Join(dir, "datos", "config.yaml"),
		DB:     filepath.Join(dir, "datos", "filebrowser.db"),
		Estado: filepath.Join(dir, "datos", "estado"),
		Cache:  filepath.Join(dir, "datos", "cache"),
		Raiz:   filepath.Join(dir, "arquivos con espazos e ñ"),
	}
	for _, d := range []string{filepath.Dir(r.Config), r.Estado, r.Cache, r.Raiz} {
		os.MkdirAll(d, 0o755)
	}
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	porto := l.Addr().(*net.TCPAddr).Port
	l.Close()

	const usuario = "admin"
	const contrasinal = `Un "contrasinal" con ñ, % e \ barras`
	base := baseLocalFB(porto)

	var saida *os.File
	arrincar := func() *exec.Cmd {
		cmd := exec.Command(bin, "-c", r.Config)
		cmd.Dir = r.Estado
		saida, _ = os.OpenFile(filepath.Join(dir, "fb.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
		cmd.Stdout, cmd.Stderr = saida, saida
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	deter := func(cmd *exec.Cmd) {
		cmd.Process.Kill()
		cmd.Wait()
		saida.Close()
		time.Sleep(500 * time.Millisecond) // que solte a base de datos
	}
	rexistro := func() string { b, _ := os.ReadFile(filepath.Join(dir, "fb.log")); return string(b) }

	// 1. Primeiro arranque co contrasinal no config.
	os.WriteFile(r.Config, []byte(xerarConfigQuantum(r, porto, usuario, contrasinal)), 0o600)
	cmd := arrincar()
	if err := agardarSaudeFB(base, 30, time.Second); err != nil {
		deter(cmd)
		t.Fatalf("%v\n%s", err, rexistro())
	}
	if err := comprobarLoginFB(base, usuario, contrasinal, 5); err != nil {
		deter(cmd)
		t.Fatalf("login co contrasinal do config: %v\n%s", err, rexistro())
	}
	if err := comprobarLoginFB(base, usuario, "outro-contrasinal-mal", 1); err == nil {
		t.Error("un contrasinal malo non debería entrar")
	}
	deter(cmd)

	// 2. Sen o contrasinal no config: o admin segue a entrar co hash da base de datos.
	os.WriteFile(r.Config, []byte(xerarConfigQuantum(r, porto, usuario, "")), 0o600)
	cmd = arrincar()
	defer deter(cmd)
	if err := agardarSaudeFB(base, 30, time.Second); err != nil {
		t.Fatalf("%v\n%s", err, rexistro())
	}
	if err := comprobarLoginFB(base, usuario, contrasinal, 5); err != nil {
		t.Fatalf("login tras quitar o contrasinal: %v\n%s", err, rexistro())
	}
}
