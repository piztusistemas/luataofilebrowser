//go:build linux

package main

// Capa de Debian (e derivadas con systemd). Ten a mesma forma ca plataforma_windows.go.
//
//   Programas:  /opt/piztu/filebrowser  (Quantum)   /opt/piztu/modulos/{tao,lua}
//   Config FB:  /etc/filebrowser/config.yaml
//   Datos FB:   /var/lib/filebrowser    (base de datos, caché, estado)
//   Arquivos:   /var/www/html/arquivos  (o que sobe o alumnado)
//   Rexistro:   /var/log/filebrowser.log
//
// File Browser corre coma servizo de systemd (piztu-filebrowser.service) coa conta sen
// privilexios www-data. Para escoitar no porto 80 recibe só CAP_NET_BIND_SERVICE.
//
// O instalador ten que ser root. Se se lanza desde o menú coma usuario normal,
// asegurarPrivilexios relánzase a si mesmo con pkexec (diálogo gráfico de contrasinal).

import (
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const (
	nomeBinFB      = "filebrowser"
	nomeBinTao     = "tao"
	nomeBinLua     = "lua"
	nomeServizoFB  = "piztu-filebrowser"
	contaServizoFB = "www-data"
	nomeRegraFW    = "Piztu File Browser"
	ficheiroUnit   = "/etc/systemd/system/" + nomeServizoFB + ".service"
	dirApps        = "/usr/share/applications"
)

func dirTao() string { return "/opt/piztu/modulos/tao" }
func dirLua() string { return "/opt/piztu/modulos/lua" }

func rutasFileBrowser() fbRutas {
	datos := "/var/lib/filebrowser"
	return fbRutas{
		DirBin:   "/opt/piztu/filebrowser",
		Bin:      "/opt/piztu/filebrowser/filebrowser",
		Config:   "/etc/filebrowser/config.yaml",
		DB:       filepath.Join(datos, "filebrowser.db"),
		Estado:   filepath.Join(datos, "estado"),
		Cache:    filepath.Join(datos, "cache"),
		Raiz:     "/var/www/html/arquivos",
		Rexistro: "/var/log/filebrowser.log",
	}
}

// Nada que engadir ao lado do binario: en Linux systemd lanza Quantum directamente.
func escribirBinariosServizoFB(fbRutas) error { return nil }

// ── Privilexios e usuario real ───────────────────────────────────────────────

// usuarioReal é quen abriu o instalador (non root). Serve para que os ficheiros que se
// escriben no seu perfil (idioma de Lúa, admin.json de Tao) non queden coma de root.
var usuarioReal *user.User

func asegurarPrivilexios() {
	if os.Geteuid() == 0 {
		for _, v := range []string{"PKEXEC_UID", "SUDO_UID", "PIZTU_UID"} {
			if u, err := user.LookupId(os.Getenv(v)); err == nil && u.Uid != "0" {
				usuarioReal = u
				break
			}
		}
		return
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "non se puido localizar o instalador:", err)
		os.Exit(1)
	}
	pk, err := exec.LookPath("pkexec")
	if err != nil {
		fmt.Fprintln(os.Stderr, "O instalador necesita permisos de administrador. Executao con: sudo", exe)
		os.Exit(1)
	}
	args := []string{"pkexec", "env", "PIZTU_UID=" + strconv.Itoa(os.Getuid())}
	for _, v := range []string{"DISPLAY", "XAUTHORITY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "LANG", "LC_ALL"} {
		if val := os.Getenv(v); val != "" {
			args = append(args, v+"="+val)
		}
	}
	args = append(args, exe)
	args = append(args, os.Args[1:]...)
	if err := syscall.Exec(pk, args, os.Environ()); err != nil {
		fmt.Fprintln(os.Stderr, "pkexec fallou:", err)
		os.Exit(1)
	}
}

// dirConfigUsuario é ~/.config de quen abriu o instalador, non o de root.
func dirConfigUsuario() (string, error) {
	if usuarioReal != nil {
		return filepath.Join(usuarioReal.HomeDir, ".config"), nil
	}
	return os.UserConfigDir()
}

// axustarPropietario pasa ruta (e o que cuelgue dela) ao usuario real.
func axustarPropietario(ruta string) {
	if usuarioReal == nil {
		return
	}
	uid, _ := strconv.Atoi(usuarioReal.Uid)
	gid, _ := strconv.Atoi(usuarioReal.Gid)
	filepath.Walk(ruta, func(p string, _ os.FileInfo, err error) error {
		if err == nil {
			os.Lchown(p, uid, gid)
		}
		return nil
	})
}

// ── Permisos ─────────────────────────────────────────────────────────────────

func idWWW() (int, int, error) {
	u, err := user.Lookup(contaServizoFB)
	if err != nil {
		return 0, 0, fmt.Errorf("non existe o usuario %s: %w", contaServizoFB, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid, nil
}

func chownRec(ruta string, uid, gid int) error {
	return filepath.Walk(ruta, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}

// protexerFicheirosFB: o config.yaml (leva o contrasinal ata o primeiro arranque) só o
// len root e o servizo; a base de datos e a caché son só do servizo (0700); os
// arquivos do alumnado son do servizo e lexibles por todos.
func protexerFicheirosFB(r fbRutas) error {
	uid, gid, err := idWWW()
	if err != nil {
		return err
	}
	dirDatos := filepath.Dir(r.DB)
	if err := chownRec(dirDatos, uid, gid); err != nil {
		return err
	}
	if err := os.Chmod(dirDatos, 0o700); err != nil {
		return err
	}
	if err := chownRec(r.Raiz, uid, gid); err != nil {
		return err
	}
	if err := os.Chmod(filepath.Dir(r.Config), 0o755); err != nil {
		return err
	}
	if err := os.Chown(r.Config, 0, gid); err != nil {
		return err
	}
	if err := os.Chmod(r.Config, 0o640); err != nil {
		return err
	}
	// O rexistro créao systemd (root) ao abrilo con append:; non fai falta máis.
	return nil
}

// ── Servizo ──────────────────────────────────────────────────────────────────

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func hayUnit() bool {
	_, err := os.Stat(ficheiroUnit)
	return err == nil
}

func detenerServizoFB() error {
	if !hayUnit() {
		return nil
	}
	return systemctl("stop", nomeServizoFB)
}

func xerarUnit(r fbRutas) string {
	return fmt.Sprintf(`# Xerado polo instalador de aula de Piztu.
[Unit]
Description=Piztu — File Browser Quantum
After=network-online.target
Wants=network-online.target

[Service]
User=%[1]s
Group=%[1]s
WorkingDirectory=%[2]s
ExecStart=%[3]s -c %[4]s
Restart=on-failure
RestartSec=5
# Porto 80 sen ser root.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
NoNewPrivileges=true
ProtectSystem=full
PrivateTmp=true
StandardOutput=append:%[5]s
StandardError=append:%[5]s

[Install]
WantedBy=multi-user.target
`, contaServizoFB, r.Estado, r.Bin, r.Config, r.Rexistro)
}

func instalarServizoFB(r fbRutas) error {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return fmt.Errorf("este equipo non usa systemd: non se pode crear o servizo")
	}
	if err := os.WriteFile(ficheiroUnit, []byte(xerarUnit(r)), 0o644); err != nil {
		return err
	}
	if err := systemctl("daemon-reload"); err != nil {
		return err
	}
	if err := systemctl("enable", nomeServizoFB); err != nil {
		return err
	}
	return systemctl("restart", nomeServizoFB)
}

func reiniciarServizoFB() error { return systemctl("restart", nomeServizoFB) }

func eliminarServizoFB() error {
	if !hayUnit() {
		return nil
	}
	exec.Command("systemctl", "disable", "--now", nomeServizoFB).Run()
	if err := os.Remove(ficheiroUnit); err != nil {
		return err
	}
	return systemctl("daemon-reload")
}

// ── Firewall ─────────────────────────────────────────────────────────────────

// ufwActivo: en Debian non hai firewall por defecto. Só se toca ufw se está instalado e
// activo; con nftables/iptables a pelo non se fai nada (a política por defecto é abrir).
func ufwActivo() bool {
	p, err := exec.LookPath("ufw")
	if err != nil {
		return false
	}
	out, err := exec.Command(p, "status").Output()
	return err == nil && strings.Contains(string(out), "Status: active")
}

func abrirFirewallFB(porto int) error {
	pecharFirewallFB()
	if !ufwActivo() {
		return nil
	}
	out, err := exec.Command("ufw", "allow", "proto", "tcp", "to", "any", "port",
		strconv.Itoa(porto), "comment", nomeRegraFW).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pecharFirewallFB() error {
	if !ufwActivo() {
		return nil
	}
	// Quita a regra de calquera dos portos que usa o instalador (se non existe, ufw
	// avisa e non importa).
	for _, p := range fbPortosCandidatos {
		exec.Command("ufw", "--force", "delete", "allow", "proto", "tcp", "to", "any", "port",
			strconv.Itoa(p), "comment", nomeRegraFW).Run()
	}
	return nil
}

// ── Accesos directos (.desktop) ──────────────────────────────────────────────

func idAtallo(nome string) string {
	r := strings.NewReplacer("ú", "u", "Ú", "u")
	return "piztu-" + strings.ToLower(r.Replace(nome))
}

func crearAtallo(nome, exe string) error {
	icona := "applications-education"
	if _, err := os.Stat(filepath.Join(filepath.Dir(exe), "logo.png")); err == nil {
		icona = filepath.Join(filepath.Dir(exe), "logo.png")
	}
	contido := fmt.Sprintf(`[Desktop Entry]
Type=Application
Name=%s
Exec="%s"
Path=%s
Icon=%s
Terminal=false
Categories=Education;
`, nome, exe, filepath.Dir(exe), icona)
	if err := os.MkdirAll(dirApps, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dirApps, idAtallo(nome)+".desktop"), []byte(contido), 0o644); err != nil {
		return err
	}
	exec.Command("update-desktop-database", dirApps).Run()
	return nil
}

func borrarAtallo(nome string) {
	os.Remove(filepath.Join(dirApps, idAtallo(nome)+".desktop"))
}

// ── Dependencias de Lúa (apt) ────────────────────────────────────────────────

var paquetesLua = []string{"pandoc", "texlive-latex-recommended", "texlive-latex-extra",
	"texlive-fonts-recommended", "texlive-xetex", "lmodern"}

func aptEnv(args ...string) *exec.Cmd {
	cmd := exec.Command("apt-get", args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd
}

func instalarDepsLua(log func(string)) error {
	if _, err := exec.LookPath("apt-get"); err != nil {
		return fmt.Errorf("non se atopou apt-get: instala pandoc e LaTeX á man")
	}
	log("  📦 apt-get update…")
	if err := executarConRexistro(aptEnv("update"), log); err != nil {
		log("  ⚠️ apt-get update: " + err.Error())
	}
	log("  📦 " + strings.Join(paquetesLua, " "))
	return executarConRexistro(aptEnv(append([]string{"install", "-y", "--no-install-recommends"}, paquetesLua...)...), log)
}

func desinstalarDepsLua(log func(string)) error {
	log("  📦 apt-get remove…")
	return executarConRexistro(aptEnv(append([]string{"remove", "-y"}, paquetesLua...)...), log)
}

func comandoOculto(n string, a ...string) *exec.Cmd { return exec.Command(n, a...) }
