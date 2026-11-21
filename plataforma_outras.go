//go:build !windows

package main

// Aínda só hai capa de Windows. O instalador de Debian (e despois o de macOS) engadirá
// aquí a súa: systemd + usuario www-data, ufw/nftables, .desktop... reutilizando
// filebrowser.go e instalar.go tal cal. Mentres tanto todo devolve un erro claro, e o
// proxecto compila e pasa os tests en calquera sistema.

import (
	"errors"
	"os/exec"
	"path/filepath"
)

var errSenSoporte = errors.New("este instalador aínda só funciona en Windows")

const (
	nomeBinFB  = "filebrowser"
	nomeBinTao = "tao"
	nomeBinLua = "lua"
)

func dirTao() string { return "/opt/piztu/modulos/tao" }
func dirLua() string { return "/opt/piztu/modulos/lua" }

func rutasFileBrowser() fbRutas {
	return fbRutas{
		DirBin: "/usr/local/bin", Bin: "/usr/local/bin/filebrowser",
		Config: "/etc/filebrowser/config.yaml", DB: "/etc/filebrowser/filebrowser.db",
		Estado: "/var/lib/filebrowser", Cache: filepath.Join("/var/lib/filebrowser", "cache"),
		Raiz: "/var/www/html/arquivos", Rexistro: "/var/log/filebrowser.log",
	}
}

func escribirBinariosServizoFB(fbRutas) error { return errSenSoporte }
func protexerFicheirosFB(fbRutas) error       { return errSenSoporte }
func detenerServizoFB() error                 { return errSenSoporte }
func instalarServizoFB(fbRutas) error         { return errSenSoporte }
func reiniciarServizoFB() error               { return errSenSoporte }
func eliminarServizoFB() error                { return errSenSoporte }
func abrirFirewallFB(int) error               { return errSenSoporte }
func pecharFirewallFB() error                 { return errSenSoporte }
func crearAtallo(string, string) error        { return errSenSoporte }
func borrarAtallo(string)                     {}
func instalarDepsLua(func(string)) error      { return errSenSoporte }
func desinstalarDepsLua(func(string)) error   { return errSenSoporte }
func comandoOculto(n string, a ...string) *exec.Cmd { return exec.Command(n, a...) }
