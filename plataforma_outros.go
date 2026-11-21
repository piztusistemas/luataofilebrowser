//go:build !windows && !linux && !darwin

package main

// Sistemas sen capa propia (FreeBSD...). Windows, Debian e macOS teñen a súa; aquí todo
// devolve un erro claro, e o proxecto compila en calquera sistema.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
)

var errSenSoporte = errors.New("este instalador só funciona en Windows, Debian e macOS")

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

func escribirBinariosServizoFB(fbRutas) error       { return errSenSoporte }
func protexerFicheirosFB(fbRutas) error             { return errSenSoporte }
func detenerServizoFB() error                       { return errSenSoporte }
func instalarServizoFB(fbRutas) error               { return errSenSoporte }
func reiniciarServizoFB() error                     { return errSenSoporte }
func eliminarServizoFB() error                      { return errSenSoporte }
func abrirFirewallFB(int) error                     { return errSenSoporte }
func pecharFirewallFB() error                       { return errSenSoporte }
func crearAtallo(string, string) error              { return errSenSoporte }
func borrarAtallo(string)                           {}
func instalarDepsLua(func(string)) error            { return errSenSoporte }
func desinstalarDepsLua(func(string)) error         { return errSenSoporte }
func comandoOculto(n string, a ...string) *exec.Cmd { return exec.Command(n, a...) }

func asegurarPrivilexios()              {}
func dirConfigUsuario() (string, error) { return os.UserConfigDir() }
func axustarPropietario(string)         {}

func comoAdministrador(tr *Translator, t tarefaAdmin, log func(string)) (map[string]any, error) {
	return executarTarefa(tr, t, log)
}
func prepararApp(string, string) error     { return errSenSoporte }
func raizApp(dir string) string            { return dir }
func configFBSenContrasinal(fbRutas) error { return errSenSoporte }
