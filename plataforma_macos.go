//go:build darwin

package main

// Capa de macOS. Ten a mesma forma ca plataforma_windows.go e plataforma_linux.go.
//
//   Tao e Lúa:  /Applications/Tao.app e /Applications/Lua.app (Lua.app é o mesmo sitio
//               que usa o instalador propio de Lúa para Mac, que queda substituído)
//   File Browser:
//     Programa:  /Library/Application Support/Piztu/FileBrowser/filebrowser
//     Config:    /Library/Application Support/Piztu/FileBrowser/config.yaml
//     Datos:     /Library/Application Support/Piztu/FileBrowser/datos (bd, caché, estado)
//     Arquivos:  /Users/Shared/Piztu/arquivos   (o que sobe o alumnado)
//     Rexistro:  /Library/Logs/Piztu/filebrowser.log
//
// File Browser corre coma LaunchDaemon de launchd (arrinca co equipo, sen sesión
// iniciada) coa conta sen privilexios _www, o equivalente ao www-data de Debian. Desde
// macOS 10.14 calquera proceso pode escoitar no porto 80, así que non fai falta nada
// parecido ao CAP_NET_BIND_SERVICE de Linux.
//
// Privilexios. A diferenza de Windows e Debian, a xanela do instalador NON corre como
// root: Homebrew (as dependencias de Lúa) négase a funcionar como root, e un .app en
// /Applications instálao calquera administrador sen elevar (igual que fai o instalador
// de Lúa). O que si precisa root (o servizo de File Browser, e Tao ou Lúa se
// /Applications non se pode escribir) faise nun proceso fillo: o propio instalador
// relanzado con «osascript … with administrator privileges», que amosa o diálogo de
// contrasinal do sistema e admite o nome e contrasinal doutro administrador. O fillo
// recibe a tarefa nun JSON e devolve o rexistro liña a liña nun ficheiro que a xanela
// vai lendo.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	nomeBinFB      = "filebrowser"
	nomeBinTao     = "tao"
	nomeBinLua     = "lua"
	contaServizoFB = "_www"
	etiquetaFB     = "gal.omeu.piztu.filebrowser"
	plistFB        = "/Library/LaunchDaemons/" + etiquetaFB + ".plist"
	dirPiztu       = "/Library/Application Support/Piztu"
	socketfilterfw = "/usr/libexec/ApplicationFirewall/socketfilterfw"
	rutaLsregister = "/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister"

	// argTarefaAdmin é o argumento co que o instalador se relanza a si mesmo coma
	// administrador (ver executarComoAdmin e asegurarPrivilexios).
	argTarefaAdmin = "--piztu-tarefa-admin"
)

// O executable vive dentro do bundle, na posición que manda a estrutura dun .app. Tao
// le tao-servidor.conf ao carón do seu executable (tao/servidor.go:
// rutasServidorConf), así que dirTao ten que ser Contents/MacOS e non o bundle.
func dirTao() string { return "/Applications/Tao.app/Contents/MacOS" }
func dirLua() string { return "/Applications/Lua.app/Contents/MacOS" }

// raizApp devolve o bundle .app que contén dir (para borralo enteiro e para amosalo).
func raizApp(dir string) string {
	if filepath.Base(dir) == "MacOS" && filepath.Base(filepath.Dir(dir)) == "Contents" {
		return filepath.Dir(filepath.Dir(dir))
	}
	return dir
}

func rutasFileBrowser() fbRutas {
	dir := filepath.Join(dirPiztu, "FileBrowser")
	datos := filepath.Join(dir, "datos")
	return fbRutas{
		DirBin:   dir,
		Bin:      filepath.Join(dir, nomeBinFB),
		Config:   filepath.Join(dir, "config.yaml"),
		DB:       filepath.Join(datos, "filebrowser.db"),
		Estado:   filepath.Join(datos, "estado"),
		Cache:    filepath.Join(datos, "cache"),
		Raiz:     "/Users/Shared/Piztu/arquivos",
		Rexistro: "/Library/Logs/Piztu/filebrowser.log",
	}
}

// Nada que engadir ao lado do binario (launchd lanza Quantum directamente). Só se lle
// quita a corentena, por se o instalador se descargou dun navegador.
func escribirBinariosServizoFB(r fbRutas) error {
	quitarCorentena(r.Bin)
	return nil
}

// ── Privilexios e usuario real ───────────────────────────────────────────────

// usuarioReal é quen abriu o instalador, cando este proceso é o fillo con permisos de
// administrador. Serve para escribir no seu perfil (idioma de Lúa, admin.json de Tao) e
// non no de root.
var usuarioReal *user.User

func asegurarPrivilexios() {
	if len(os.Args) == 3 && os.Args[1] == argTarefaAdmin {
		os.Exit(executarTarefaFillo(os.Args[2]))
	}
	if os.Geteuid() == 0 {
		for _, v := range []string{"PIZTU_UID", "SUDO_UID"} {
			if u, err := user.LookupId(os.Getenv(v)); err == nil && u.Uid != "0" {
				usuarioReal = u
				break
			}
		}
	}
	// Abrindo o .app desde o Finder, launchd dá un PATH mínimo sen Homebrew nin TeX.
	refrescarPath()
}

// dirConfigUsuario é ~/Library/Application Support de quen abriu o instalador: o
// mesmo os.UserConfigDir() que usan Tao e Lúa en macOS (non ~/.config).
func dirConfigUsuario() (string, error) {
	if usuarioReal != nil {
		return filepath.Join(usuarioReal.HomeDir, "Library", "Application Support"), nil
	}
	return os.UserConfigDir()
}

// axustarPropietario pasa ruta (e o que colgue dela) ao usuario real.
func axustarPropietario(ruta string) {
	if usuarioReal == nil {
		return
	}
	uid, _ := strconv.Atoi(usuarioReal.Uid)
	gid, _ := strconv.Atoi(usuarioReal.Gid)
	chownRec(ruta, uid, gid)
}

func chownRec(ruta string, uid, gid int) error {
	return filepath.Walk(ruta, func(p string, _ os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		return os.Lchown(p, uid, gid)
	})
}

// ── Tarefas con permisos de administrador ────────────────────────────────────

// comoAdministrador fai a tarefa neste proceso se pode, ou senón nun fillo con
// permisos de administrador. LaTeX e Pandoc (Homebrew) instálanse sempre neste
// proceso, coma o usuario: Homebrew négase a funcionar como root.
func comoAdministrador(tr *Translator, t tarefaAdmin, log func(string)) (map[string]any, error) {
	if os.Geteuid() == 0 || !necesitaAdmin(t) {
		return executarTarefa(tr, t, log)
	}
	deps := t.Deps && (t.Nome == tarefaLuaInstalar || t.Nome == tarefaLuaDesinstalar)
	t.Deps = false
	extra, err := executarComoAdmin(tr, t, log)
	if err != nil {
		return nil, err
	}
	switch t.Nome {
	case tarefaTaoInstalar:
		rexistrarApp(raizApp(dirTao())) // na base de Launch Services do usuario, non na de root
	case tarefaLuaInstalar:
		rexistrarApp(raizApp(dirLua()))
		if deps {
			pasoDepsLua(tr, log)
		}
	case tarefaLuaDesinstalar:
		if deps {
			if err := desinstalarDepsLua(log); err != nil {
				log("⚠️ " + err.Error())
			}
		}
	}
	return extra, nil
}

// necesitaAdmin: File Browser sempre (launchd, /Library, firewall). Tao e Lúa só se
// non se pode escribir en /Applications ou no bundle que xa hai (instalado por outra
// conta, por exemplo).
func necesitaAdmin(t tarefaAdmin) bool {
	var dir string
	switch t.Nome {
	case tarefaTaoInstalar, tarefaTaoDesinstalar:
		dir = dirTao()
	case tarefaLuaInstalar, tarefaLuaDesinstalar:
		dir = dirLua()
	default:
		return true
	}
	if !podeEscribirEn("/Applications") {
		return true
	}
	for _, d := range []string{raizApp(dir), filepath.Join(raizApp(dir), "Contents"), dir} {
		if _, err := os.Stat(d); err == nil && !podeEscribirEn(d) {
			return true
		}
	}
	return false
}

// podeEscribirEn próbao de verdade: en macOS os bits de permiso non o din todo (SIP,
// ACL, volumes de só lectura).
func podeEscribirEn(dir string) bool {
	proba, err := os.MkdirTemp(dir, ".piztu-proba-")
	if err != nil {
		return false
	}
	os.RemoveAll(proba)
	return true
}

type resultadoAdmin struct {
	Erro  string         `json:"erro"`
	Extra map[string]any `json:"extra"`
}

// comiñasShell protexe s para /bin/sh (o de «do shell script»).
func comiñasShell(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// executarComoAdmin relanza este mesmo executable coma root co diálogo de contrasinal
// do sistema, e pasa ao log o que o fillo vai escribindo. A tarefa (co contrasinal de
// File Browser, se o leva) vai nun ficheiro 0600 dun cartafol 0700, nunca na liña de
// comandos, que ve calquera usuario con ps; o fillo bórrao en canto o le.
func executarComoAdmin(tr *Translator, t tarefaAdmin, log func(string)) (map[string]any, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	if e, err := filepath.EvalSymlinks(exe); err == nil {
		exe = e
	}
	dir, err := os.MkdirTemp("", "piztu-admin-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	if err := os.Chmod(dir, 0o700); err != nil {
		return nil, err
	}
	datos, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(dir, "tarefa.json"), datos, 0o600); err != nil {
		return nil, err
	}
	rutaRex := filepath.Join(dir, "rexistro.log")
	if err := os.WriteFile(rutaRex, nil, 0o600); err != nil {
		return nil, err
	}

	log(tr.T("admin.log.pedindo"))
	orde := fmt.Sprintf("PIZTU_UID=%d %s %s %s </dev/null >/dev/null 2>&1",
		os.Getuid(), comiñasShell(exe), argTarefaAdmin, comiñasShell(dir))
	cmd := exec.Command("/usr/bin/osascript",
		"-e", "on run argv",
		"-e", "do shell script (item 1 of argv) with prompt (item 2 of argv) with administrator privileges",
		"-e", "end run",
		orde, tr.T("admin.prompt"))
	var saida bytes.Buffer
	cmd.Stdout, cmd.Stderr = &saida, &saida
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	rematou := make(chan error, 1)
	go func() { rematou <- cmd.Wait() }()

	// Vai lendo o rexistro do fillo mentres traballa.
	f, err := os.Open(rutaRex)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lector := bufio.NewReader(f)
	var anaco string
	ler := func() {
		for {
			l, err := lector.ReadString('\n')
			anaco += l
			if err != nil {
				return // io.EOF: aínda non hai máis
			}
			log(strings.TrimRight(anaco, "\r\n"))
			anaco = ""
		}
	}
	var errCmd error
	for fin := false; !fin; {
		select {
		case errCmd = <-rematou:
			fin = true
		case <-time.After(300 * time.Millisecond):
		}
		ler()
	}
	if anaco != "" {
		log(anaco)
	}

	var res resultadoAdmin
	datos, errRes := os.ReadFile(filepath.Join(dir, "resultado.json"))
	if errRes != nil {
		texto := strings.TrimSpace(saida.String())
		if strings.Contains(texto, "-128") { // «User canceled»
			return nil, errors.New(tr.T("admin.cancelado"))
		}
		if errCmd != nil {
			return nil, fmt.Errorf("%s: %v %s", tr.T("admin.fallou"), errCmd, texto)
		}
		return nil, errors.New(tr.T("admin.fallou"))
	}
	if err := json.Unmarshal(datos, &res); err != nil {
		return nil, err
	}
	if res.Erro != "" {
		return nil, errors.New(res.Erro)
	}
	return res.Extra, nil
}

// executarTarefaFillo é o lado root de executarComoAdmin. Devolve o código de saída.
func executarTarefaFillo(dir string) int {
	if os.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, argTarefaAdmin+" só se usa desde o propio instalador")
		return 2
	}
	if u, err := user.LookupId(os.Getenv("PIZTU_UID")); err == nil && u.Uid != "0" {
		usuarioReal = u
	}
	refrescarPath()

	rex, err := os.OpenFile(filepath.Join(dir, "rexistro.log"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		return 1
	}
	defer rex.Close()
	log := func(l string) { fmt.Fprintln(rex, strings.TrimRight(l, "\r\n")) }

	res := resultadoAdmin{}
	rutaT := filepath.Join(dir, "tarefa.json")
	datos, err := os.ReadFile(rutaT)
	os.Remove(rutaT) // leva o contrasinal de File Browser
	var t tarefaAdmin
	if err == nil {
		err = json.Unmarshal(datos, &t)
	}
	if err == nil {
		res.Extra, err = executarTarefa(NewTranslator(t.Idioma), t, log)
	}
	if err != nil {
		res.Erro = err.Error()
	}
	saida, _ := json.Marshal(res)
	if err := os.WriteFile(filepath.Join(dir, "resultado.json"), saida, 0o644); err != nil {
		return 1
	}
	return 0
}

// ── Permisos ─────────────────────────────────────────────────────────────────

func idConta(nome string) (int, int, error) {
	u, err := user.Lookup(nome)
	if err != nil {
		return 0, 0, fmt.Errorf("non existe o usuario %s: %w", nome, err)
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid, nil
}

// protexerFicheirosFB: o mesmo esquema ca en Debian. O programa e o config.yaml son de
// root (o config só o le o servizo mentres leva o contrasinal); a base de datos e a
// caché son só do servizo (0700); os arquivos do alumnado son do servizo e lexibles por
// todos, tamén polo profesorado desde o Finder.
func protexerFicheirosFB(r fbRutas) error {
	uid, gid, err := idConta(contaServizoFB)
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
	// O rexistro créase xa do servizo, por se launchd o abre coma _www.
	if err := os.MkdirAll(filepath.Dir(r.Rexistro), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(r.Rexistro, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return err
	}
	f.Close()
	return os.Chown(r.Rexistro, uid, gid)
}

// configFBSenContrasinal: o config.yaml xa non leva o contrasinal, así que pode lelo
// todo o mundo. Fai falta porque a xanela do instalador non corre como root e le del o
// porto (portoInstalado) para saber se File Browser está instalado.
func configFBSenContrasinal(r fbRutas) error { return os.Chmod(r.Config, 0o644) }

// ── Servizo (launchd) ────────────────────────────────────────────────────────

func launchctl(args ...string) error {
	out, err := exec.Command("/bin/launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

func servizoCargado() bool {
	return exec.Command("/bin/launchctl", "print", "system/"+etiquetaFB).Run() == nil
}

func detenerServizoFB() error {
	if !servizoCargado() {
		return nil
	}
	if err := launchctl("bootout", "system/"+etiquetaFB); err != nil && servizoCargado() {
		return err
	}
	// bootout pode volver antes de que o proceso remate de saír.
	for i := 0; i < 40 && servizoCargado(); i++ {
		time.Sleep(250 * time.Millisecond)
	}
	return nil
}

func xmlTexto(s string) string {
	var b strings.Builder
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

func xerarPlistFB(r fbRutas) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<!-- Xerado polo instalador de aula de Piztu. -->
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%[1]s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%[2]s</string>
		<string>-c</string>
		<string>%[3]s</string>
	</array>
	<key>WorkingDirectory</key>
	<string>%[4]s</string>
	<key>UserName</key>
	<string>%[5]s</string>
	<key>GroupName</key>
	<string>%[5]s</string>
	<key>RunAtLoad</key>
	<true/>
	<!-- Se cae, launchd volve arrincalo (o Restart=on-failure de systemd). -->
	<key>KeepAlive</key>
	<dict>
		<key>SuccessfulExit</key>
		<false/>
	</dict>
	<key>ThrottleInterval</key>
	<integer>5</integer>
	<key>StandardOutPath</key>
	<string>%[6]s</string>
	<key>StandardErrorPath</key>
	<string>%[6]s</string>
</dict>
</plist>
`, etiquetaFB, xmlTexto(r.Bin), xmlTexto(r.Config), xmlTexto(r.Estado), contaServizoFB, xmlTexto(r.Rexistro))
}

func instalarServizoFB(r fbRutas) error {
	if err := detenerServizoFB(); err != nil {
		return err
	}
	if err := os.WriteFile(plistFB, []byte(xerarPlistFB(r)), 0o644); err != nil {
		return err
	}
	os.Chown(plistFB, 0, 0)
	// Un servizo desactivado (launchctl disable) non se carga aínda que exista o plist.
	launchctl("enable", "system/"+etiquetaFB)
	var err error
	for i := 0; i < 5; i++ {
		// Xusto despois dun bootout, bootstrap pode fallar un intre con «Input/output
		// error» mentres launchd remata de descargar o servizo vello.
		if err = launchctl("bootstrap", "system", plistFB); err == nil || servizoCargado() {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}

func reiniciarServizoFB() error { return launchctl("kickstart", "-k", "system/"+etiquetaFB) }

func eliminarServizoFB() error {
	if err := detenerServizoFB(); err != nil {
		return err
	}
	if err := os.Remove(plistFB); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// ── Firewall ─────────────────────────────────────────────────────────────────

// O firewall de macOS non vai por portos senón por aplicacións: se está activo, bloquea
// as conexións entrantes de calquera programa que non estea na súa lista. Para un
// servizo sen xanela nin sequera pregunta, así que hai que engadilo a man. Se o
// firewall está apagado (o habitual), engadilo non cambia nada.
func abrirFirewallFB(int) error {
	bin := rutasFileBrowser().Bin
	pecharFirewallFB()
	for _, args := range [][]string{{"--add", bin}, {"--unblockapp", bin}} {
		if out, err := exec.Command(socketfilterfw, args...).CombinedOutput(); err != nil {
			return fmt.Errorf("socketfilterfw %s: %v: %s", args[0], err, strings.TrimSpace(string(out)))
		}
	}
	return nil
}

func pecharFirewallFB() error {
	exec.Command(socketfilterfw, "--remove", rutasFileBrowser().Bin).Run()
	return nil
}

// ── Bundles .app («accesos directos») ────────────────────────────────────────

// prepararApp converte o cartafol do executable nun bundle .app: Info.plist, icona e
// sen corentena. Sen o Info.plist o Finder non o recoñece coma aplicación, así que isto
// non é cosmética e un erro aquí aborta a instalación.
//
// O bundle non se volve asinar: o executable xa vén asinado (ad hoc) da súa Release, e
// iso é o que esixe macOS para arrincalo (comprobado). Unha sinatura do bundle enteiro
// non duraría: Tao e Lúa crean modulos/ en Contents/MacOS ao arrincar, e o instalador
// escribe alí tao-servidor.conf.
func prepararApp(nome, exe string) error {
	app := raizApp(filepath.Dir(exe))
	id := filepath.Base(exe) // "tao" ou "lua"
	cont := filepath.Join(app, "Contents")
	if err := os.WriteFile(filepath.Join(cont, "Info.plist"), []byte(xerarInfoPlist(nome, id, versionEmbebida(nome))), 0o644); err != nil {
		return err
	}
	os.WriteFile(filepath.Join(cont, "PkgInfo"), []byte("APPL????"), 0o644)
	if icona, err := lerEmbebido(id + ".icns"); err == nil {
		res := filepath.Join(cont, "Resources")
		if err := os.MkdirAll(res, 0o755); err == nil {
			os.WriteFile(filepath.Join(res, "icons.icns"), icona, 0o644)
		}
	}
	quitarCorentena(app)
	// Se o instala o fillo root, que siga sendo de quen abriu o instalador: así pode
	// actualizalo el mesmo (Tao e Lúa actualízanse sós desde a súa xanela).
	axustarPropietario(app)
	return nil
}

func xerarInfoPlist(nome, id, version string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleName</key>
	<string>%[1]s</string>
	<key>CFBundleDisplayName</key>
	<string>%[1]s</string>
	<key>CFBundleExecutable</key>
	<string>%[2]s</string>
	<key>CFBundleIdentifier</key>
	<string>gal.omeu.piztu.%[2]s</string>
	<key>CFBundleVersion</key>
	<string>%[3]s</string>
	<key>CFBundleShortVersionString</key>
	<string>%[3]s</string>
	<key>CFBundleIconFile</key>
	<string>icons</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>LSApplicationCategoryType</key>
	<string>public.app-category.education</string>
	<key>NSHumanReadableCopyright</key>
	<string>Copyright © Piztu</string>
</dict>
</plist>
`, xmlTexto(nome), xmlTexto(id), xmlTexto(version))
}

// versionEmbebida le a versión de nome do versions.txt que deixa build-macos.sh.
func versionEmbebida(nome string) string {
	datos, _ := lerEmbebido("versions.txt")
	for _, l := range strings.Split(string(datos), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(l), nome+" "); ok {
			return strings.TrimSpace(v)
		}
	}
	return "1.0"
}

func quitarCorentena(ruta string) {
	exec.Command("/usr/bin/xattr", "-dr", "com.apple.quarantine", ruta).Run()
}

func rexistrarApp(app string) { exec.Command(rutaLsregister, "-f", app).Run() }

func appDeNome(nome string) string {
	if nome == "Tao" {
		return raizApp(dirTao())
	}
	return raizApp(dirLua())
}

// crearAtallo: un .app en /Applications xa sae en Launchpad, Spotlight e o Finder; só
// se rexistra en Launch Services para que apareza decontado.
func crearAtallo(nome, exe string) error {
	rexistrarApp(raizApp(filepath.Dir(exe)))
	return nil
}

func borrarAtallo(nome string) { exec.Command(rutaLsregister, "-u", appDeNome(nome)).Run() }

// ── PATH ─────────────────────────────────────────────────────────────────────

// refrescarPath engade ao PATH os sitios de Homebrew e dos .pkg (MacTeX). Mesma
// solución ca lua_installer/macos_darwin.go: launchd dálle a unha app aberta desde o
// Finder só /usr/bin:/bin:/usr/sbin:/sbin.
func refrescarPath() {
	var novo []string
	vistos := map[string]bool{}
	engadir := func(dir string, comprobar bool) {
		dir = strings.TrimSpace(dir)
		if dir == "" || vistos[dir] || strings.HasPrefix(dir, "#") {
			return
		}
		if comprobar {
			if st, err := os.Stat(dir); err != nil || !st.IsDir() {
				return
			}
		}
		vistos[dir] = true
		novo = append(novo, dir)
	}
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		engadir(d, false)
	}
	// As mesmas fontes que le path_helper: por aí se engade MacTeX (/etc/paths.d/TeX).
	ficheiros := []string{"/etc/paths"}
	if e, err := os.ReadDir("/etc/paths.d"); err == nil {
		for _, f := range e {
			if !f.IsDir() {
				ficheiros = append(ficheiros, filepath.Join("/etc/paths.d", f.Name()))
			}
		}
	}
	for _, f := range ficheiros {
		if datos, err := os.ReadFile(f); err == nil {
			for _, l := range strings.Split(string(datos), "\n") {
				engadir(l, true)
			}
		}
	}
	for _, d := range []string{"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin",
		"/usr/local/sbin", "/Library/TeX/texbin"} {
		engadir(d, true)
	}
	os.Setenv("PATH", strings.Join(novo, string(os.PathListSeparator)))
}

// ── Dependencias de Lúa (Homebrew) ───────────────────────────────────────────
//
// As mesmas que o instalador propio de Lúa para Mac (lua_installer/macos_darwin.go):
// poppler (pdftoppm/pdftotext) e pandoc coma fórmulas, e MacTeX sen as apps de
// escritorio coma cask. É o único cask que trae os tres motores que Lúa ofrece
// (pdflatex, xelatex e lualatex) e TikZ.

var formulasLua = []string{"poppler", "pandoc"}

const caskLatex = "mactex-no-gui"

const urlInstaladorHomebrew = "https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh"

func existeOrde(nome string) bool {
	_, err := exec.LookPath(nome)
	return err == nil
}

// entornoAskpass: un cask .pkg (MacTeX) e o script de Homebrew chaman a sudo por
// dentro; sen terminal, sudo só pode pedir o contrasinal a través de SUDO_ASKPASS
// (Homebrew engade o -A el só). O contrasinal vai do diálogo de osascript a sudo sen
// pasar polo instalador; o script vive nun cartafol 0700 para que ninguén o cambie.
func entornoAskpass(mensaxe string) ([]string, func()) {
	dir, err := os.MkdirTemp("", "piztu-askpass-")
	if err != nil {
		return os.Environ(), func() {}
	}
	limpar := func() { os.RemoveAll(dir) }
	esc := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", " ", "'", "’").Replace(mensaxe)
	guion := "#!/bin/sh\n/usr/bin/osascript \\\n" +
		"  -e 'display dialog \"" + esc + "\" with title \"Instalador de aula\" default answer \"\" " +
		"with hidden answer buttons {\"Cancelar\", \"Aceptar\"} default button \"Aceptar\" with icon caution' \\\n" +
		"  -e 'text returned of result'\n"
	ruta := filepath.Join(dir, "askpass")
	if os.Chmod(dir, 0o700) != nil || os.WriteFile(ruta, []byte(guion), 0o700) != nil {
		limpar()
		return os.Environ(), func() {}
	}
	return append(os.Environ(), "SUDO_ASKPASS="+ruta), limpar
}

// entornoBrew: sen o «brew update» automático (minutos en silencio) nin consellos.
func entornoBrew(env []string) []string {
	return append(env, "HOMEBREW_NO_AUTO_UPDATE=1", "HOMEBREW_NO_ENV_HINTS=1")
}

func asegurarHomebrew(log func(string)) error {
	if existeOrde("brew") {
		return nil
	}
	log("  📦 Este Mac non ten Homebrew: instalándoo desde brew.sh (pode tardar varios minutos)…")
	env, limpar := entornoAskpass("O instalador de aula precisa permisos de administrador para instalar Homebrew, o xestor de paquetes que instala LaTeX e Pandoc.")
	defer limpar()
	cmd := exec.Command("/bin/bash", "-c", "set -o pipefail; /usr/bin/curl -fsSL "+urlInstaladorHomebrew+" | /bin/bash")
	// NONINTERACTIVE: sen el o script queda agardando un RETURN que nunca chega.
	cmd.Env = append(env, "NONINTERACTIVE=1")
	errCmd := executarConRexistro(cmd, log)
	refrescarPath()
	if !existeOrde("brew") {
		return fmt.Errorf("non se puido instalar Homebrew (%v): instálao á man desde https://brew.sh e volve executar este paso", errCmd)
	}
	return nil
}

// progresoBrew: a descarga de MacTeX son uns 6 GB e Homebrew, sen terminal, non escribe
// nada mentres tanto. Cada 30 s dise canto leva, para que ninguén peche o instalador
// pensando que se colgou.
func progresoBrew(log func(string)) func() {
	feito, rematado := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(rematado)
		inicio := time.Now()
		tic := time.NewTicker(30 * time.Second)
		defer tic.Stop()
		cache := ""
		if out, err := exec.Command("brew", "--cache").Output(); err == nil {
			cache = strings.TrimSpace(string(out))
		}
		for {
			select {
			case <-feito:
				return
			case <-tic.C:
				min := int(time.Since(inicio).Minutes())
				if mb := mbDescargados(cache); mb > 0 {
					log(fmt.Sprintf("   ⏳ %d min — descargados %d MB…", min, mb))
				} else {
					log(fmt.Sprintf("   ⏳ %d min — traballando…", min))
				}
			}
		}
	}()
	return func() { close(feito); <-rematado }
}

func mbDescargados(cache string) int64 {
	if cache == "" {
		return 0
	}
	ficheiros, _ := filepath.Glob(filepath.Join(cache, "downloads", "*.incomplete"))
	var mellor os.FileInfo
	for _, f := range ficheiros {
		if st, err := os.Stat(f); err == nil && (mellor == nil || st.ModTime().After(mellor.ModTime())) {
			mellor = st
		}
	}
	if mellor == nil {
		return 0
	}
	return mellor.Size() / (1024 * 1024)
}

func instalarDepsLua(log func(string)) error {
	if os.Geteuid() == 0 {
		return errors.New("Homebrew non funciona como root: abre o instalador cun dobre clic, sen sudo")
	}
	if err := asegurarHomebrew(log); err != nil {
		return err
	}
	var ultimo error

	log("  📦 " + strings.Join(formulasLua, ", ") + " (Homebrew)…")
	cmd := exec.Command("brew", append([]string{"install"}, formulasLua...)...)
	cmd.Env = entornoBrew(os.Environ())
	parar := progresoBrew(log)
	if err := executarConRexistro(cmd, log); err != nil {
		ultimo = fmt.Errorf("%s: %w", strings.Join(formulasLua, ", "), err)
	}
	parar()

	log("  📦 " + caskLatex + " (MacTeX, uns 6 GB: entre 10 e 45 minutos segundo a conexión; non peches o instalador)…")
	env, limpar := entornoAskpass("O instalador de aula precisa permisos de administrador para instalar MacTeX (LaTeX).")
	defer limpar()
	cmd = exec.Command("brew", "install", "--cask", caskLatex)
	cmd.Env = entornoBrew(env)
	parar = progresoBrew(log)
	if err := executarConRexistro(cmd, log); err != nil {
		ultimo = fmt.Errorf("%s: %w", caskLatex, err)
	}
	parar()
	refrescarPath()
	return ultimo
}

func desinstalarDepsLua(log func(string)) error {
	if !existeOrde("brew") {
		return errors.New("non se atopou Homebrew: LaTeX e Pandoc, se os hai, instaláronse por outra vía")
	}
	var ultimo error
	log("  📦 brew uninstall " + strings.Join(formulasLua, " ") + "…")
	if err := executarConRexistro(exec.Command("brew", append([]string{"uninstall", "--ignore-dependencies"}, formulasLua...)...), log); err != nil {
		ultimo = err
	}
	log("  📦 brew uninstall --cask " + caskLatex + "…")
	env, limpar := entornoAskpass("O instalador de aula precisa permisos de administrador para desinstalar MacTeX.")
	defer limpar()
	cmd := exec.Command("brew", "uninstall", "--cask", caskLatex)
	cmd.Env = env
	if err := executarConRexistro(cmd, log); err != nil {
		ultimo = err
	}
	return ultimo
}

func comandoOculto(n string, a ...string) *exec.Cmd { return exec.Command(n, a...) }
