//go:build windows

package main

// Capa de Windows. O instalador xa corre elevado: o manifesto
// (build/windows/wails.exe.manifest) leva requireAdministrator e Windows pide UAC antes
// de que main() empece.
//
//   Programas:  %ProgramFiles%\Piztu\{FileBrowser,Tao,Lua}
//   Datos FB:   %ProgramData%\Piztu\FileBrowser  (config.yaml, base de datos, caché, rexistro)
//   Arquivos:   %ProgramData%\Piztu\arquivos     (o que sobe o alumnado)
//
// File Browser corre coma servizo de Windows co usuario LocalService (sen privilexios,
// o equivalente ao www-data de Debian). Quantum non sabe falar co xestor de servizos de
// Windows, así que o servizo é fbservizo.exe (cmd/fbservizo), que o lanza e o detén.

import (
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	nomeBinFB       = "filebrowser.exe"
	nomeBinServizo  = "fbservizo.exe"
	nomeBinTao      = "tao.exe"
	nomeBinLua      = "lua.exe"
	nomeServizoFB   = "PiztuFileBrowser"
	nomeRegraFW     = "Piztu File Browser"
	contaServizoFB  = `NT AUTHORITY\LocalService`
	sidLocalService = "*S-1-5-19"
	sidSystem       = "*S-1-5-18"
	sidAdmins       = "*S-1-5-32-544"
)

func dirProgramas() string {
	pf := os.Getenv("ProgramFiles")
	if pf == "" {
		pf = `C:\Program Files`
	}
	return filepath.Join(pf, "Piztu")
}

func dirDatosComuns() string {
	pd := os.Getenv("ProgramData")
	if pd == "" {
		pd = `C:\ProgramData`
	}
	return filepath.Join(pd, "Piztu")
}

func dirTao() string { return filepath.Join(dirProgramas(), "Tao") }

// dirLua é o mesmo que usa o instalador propio de Lúa (lua_installer), para que un
// substitúa ao outro sen deixar dúas copias.
func dirLua() string { return filepath.Join(dirProgramas(), "Lua") }

func rutasFileBrowser() fbRutas {
	datos := filepath.Join(dirDatosComuns(), "FileBrowser")
	bin := filepath.Join(dirProgramas(), "FileBrowser")
	return fbRutas{
		DirBin:   bin,
		Bin:      filepath.Join(bin, nomeBinFB),
		Config:   filepath.Join(datos, "config.yaml"),
		DB:       filepath.Join(datos, "filebrowser.db"),
		Estado:   filepath.Join(datos, "estado"),
		Cache:    filepath.Join(datos, "cache"),
		Raiz:     filepath.Join(dirDatosComuns(), "arquivos"),
		Rexistro: filepath.Join(datos, "filebrowser.log"),
	}
}

func escribirBinariosServizoFB(r fbRutas) error {
	return escribirEmbebido(nomeBinServizo, filepath.Join(r.DirBin, nomeBinServizo))
}

// ── Permisos (ACL) ───────────────────────────────────────────────────────────
//
// Con SID e non con nomes: en Windows en galego ou castelán "Administrators" e
// "LOCAL SERVICE" chámanse doutro xeito e icacls non os atoparía.

func icacls(args ...string) error {
	out, err := comandoOculto("icacls", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("icacls %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// protexerFicheirosFB: o cartafol de datos fica só para SYSTEM, Administradores e o
// servizo (o alumnado e os outros usuarios do equipo non len a base de datos nin o
// config.yaml, que leva o contrasinal ata o primeiro arranque). O servizo pode
// modificar datos e arquivos.
func protexerFicheirosFB(r fbRutas) error {
	dirDatos := filepath.Dir(r.Config)
	if err := icacls(dirDatos, "/inheritance:r",
		"/grant:r", sidSystem+":(OI)(CI)F",
		"/grant:r", sidAdmins+":(OI)(CI)F",
		"/grant:r", sidLocalService+":(OI)(CI)M"); err != nil {
		return err
	}
	if err := icacls(r.Config, "/inheritance:r",
		"/grant:r", sidSystem+":F",
		"/grant:r", sidAdmins+":F",
		"/grant:r", sidLocalService+":R"); err != nil {
		return err
	}
	return icacls(r.Raiz, "/grant", sidLocalService+":(OI)(CI)M")
}

// ── Servizo ──────────────────────────────────────────────────────────────────

func conectarSCM() (*mgr.Mgr, error) {
	m, err := mgr.Connect()
	if err != nil {
		return nil, fmt.Errorf("non se pode abrir o xestor de servizos: %w", err)
	}
	return m, nil
}

// agardarEstado agarda a que o servizo chegue a quere (ou a que pase o tempo).
func agardarEstado(s *mgr.Service, quere svc.State, tempo time.Duration) error {
	limite := time.Now().Add(tempo)
	for time.Now().Before(limite) {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == quere {
			return nil
		}
		time.Sleep(300 * time.Millisecond)
	}
	return fmt.Errorf("o servizo %s non chegou ao estado %d", nomeServizoFB, quere)
}

func detenerServizoFB() error {
	m, err := conectarSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(nomeServizoFB)
	if err != nil {
		return nil // non existe
	}
	defer s.Close()
	if st, err := s.Query(); err == nil && st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return err
	}
	return agardarEstado(s, svc.Stopped, 20*time.Second)
}

// instalarServizoFB crea o servizo (ou actualiza o existente) e arríncao.
func instalarServizoFB(r fbRutas) error {
	m, err := conectarSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()

	exe := filepath.Join(r.DirBin, nomeBinServizo)
	args := []string{"-fb", r.Bin, "-c", r.Config, "-dir", r.Estado, "-log", r.Rexistro}
	cfg := mgr.Config{
		DisplayName:      "Piztu — File Browser Quantum",
		Description:      "Servidor de arquivos da aula (Tao). Instalado polo instalador de aula de Piztu.",
		StartType:        mgr.StartAutomatic,
		ServiceStartName: contaServizoFB,
	}

	s, err := m.OpenService(nomeServizoFB)
	if err == nil {
		// Xa existía: actualizar ruta, argumentos e conta.
		actual, err := s.Config()
		if err != nil {
			s.Close()
			return err
		}
		actual.BinaryPathName = comandoServizo(exe, args)
		actual.DisplayName, actual.Description = cfg.DisplayName, cfg.Description
		actual.StartType, actual.ServiceStartName = cfg.StartType, cfg.ServiceStartName
		if err := s.UpdateConfig(actual); err != nil {
			s.Close()
			return err
		}
	} else {
		s, err = m.CreateService(nomeServizoFB, exe, cfg, args...)
		if err != nil {
			return err
		}
	}
	defer s.Close()

	// Se cae, Windows volve arrincalo (o Restart=on-failure de systemd).
	s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}, 24*60*60)

	if err := s.Start(); err != nil {
		return err
	}
	return agardarEstado(s, svc.Running, 20*time.Second)
}

// comandoServizo reproduce como compón mgr.CreateService a liña do servizo.
func comandoServizo(exe string, args []string) string {
	partes := []string{syscall.EscapeArg(exe)}
	for _, a := range args {
		partes = append(partes, syscall.EscapeArg(a))
	}
	return strings.Join(partes, " ")
}

func reiniciarServizoFB() error {
	if err := detenerServizoFB(); err != nil {
		return err
	}
	m, err := conectarSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(nomeServizoFB)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Start(); err != nil {
		return err
	}
	return agardarEstado(s, svc.Running, 20*time.Second)
}

func eliminarServizoFB() error {
	detenerServizoFB()
	m, err := conectarSCM()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	s, err := m.OpenService(nomeServizoFB)
	if err != nil {
		return nil // xa non estaba
	}
	defer s.Close()
	return s.Delete()
}

// ── Firewall ─────────────────────────────────────────────────────────────────

// abrirFirewallFB abre o porto en TODOS os perfís de rede: nas aulas a wifi adoita
// quedar coma rede "Pública", e unha regra só para "Privada" deixaría fóra o alumnado.
func abrirFirewallFB(porto int) error {
	pecharFirewallFB()
	out, err := comandoOculto("netsh", "advfirewall", "firewall", "add", "rule",
		"name="+nomeRegraFW, "dir=in", "action=allow", "protocol=TCP",
		"localport="+strconv.Itoa(porto), "profile=any").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func pecharFirewallFB() error {
	// Se a regra non existe netsh devolve erro: non importa.
	comandoOculto("netsh", "advfirewall", "firewall", "delete", "rule", "name="+nomeRegraFW).Run()
	return nil
}

// ── Accesos directos ─────────────────────────────────────────────────────────

func dirMenuInicio() string {
	return filepath.Join(dirDatosComunsSistema(), "Microsoft", "Windows", "Start Menu", "Programs")
}

func dirDatosComunsSistema() string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		return pd
	}
	return `C:\ProgramData`
}

// dirEscritorio pregúntalle a Windows onde está o escritorio do usuario (pode estar
// redirixido a OneDrive), en vez de supoñer %USERPROFILE%\Desktop.
func dirEscritorio() string {
	if d, err := windows.KnownFolderPath(windows.FOLDERID_Desktop, 0); err == nil {
		return d
	}
	h, _ := os.UserHomeDir()
	return filepath.Join(h, "Desktop")
}

// crearAtallo crea «nome.lnk» no menú Inicio de todos os usuarios e no escritorio de
// quen instala. O de escritorio non é crítico.
func crearAtallo(nome, exe string) error {
	if err := crearLnk(filepath.Join(dirMenuInicio(), nome+".lnk"), exe); err != nil {
		return err
	}
	crearLnk(filepath.Join(dirEscritorio(), nome+".lnk"), exe)
	return nil
}

func borrarAtallo(nome string) {
	os.Remove(filepath.Join(dirMenuInicio(), nome+".lnk"))
	os.Remove(filepath.Join(dirEscritorio(), nome+".lnk"))
}

// crearLnk usa WScript.Shell desde PowerShell (mesmo método ca lua_installer).
func crearLnk(lnk, destino string) error {
	if err := os.MkdirAll(filepath.Dir(lnk), 0o755); err != nil {
		return err
	}
	esc := func(s string) string { return strings.ReplaceAll(s, "'", "''") }
	script := fmt.Sprintf(
		"$s = New-Object -ComObject WScript.Shell; $sc = $s.CreateShortcut('%s'); "+
			"$sc.TargetPath = '%s'; $sc.WorkingDirectory = '%s'; $sc.IconLocation = '%s,0'; $sc.Save()",
		esc(lnk), esc(destino), esc(filepath.Dir(destino)), esc(destino))
	out, err := comandoOculto("powershell", argsPowerShell(script)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// argsPowerShell pasa o script en UTF-16LE + base64: o til de «Lúa.lnk» chega intacto
// sexa cal sexa a páxina de códigos da consola.
func argsPowerShell(script string) []string {
	u := utf16.Encode([]rune(script))
	b := make([]byte, 2*len(u))
	for i, c := range u {
		b[2*i], b[2*i+1] = byte(c), byte(c>>8)
	}
	return []string{"-NoProfile", "-NonInteractive", "-EncodedCommand", base64.StdEncoding.EncodeToString(b)}
}

// ── Dependencias de Lúa (winget) ─────────────────────────────────────────────

// Mesmos paquetes que lua_installer e o botón de Opcións de Lúa.
var paquetesLua = []string{"MiKTeX.MiKTeX", "JohnMacFarlane.Pandoc"}

func instalarDepsLua(log func(string)) error {
	if _, err := exec.LookPath("winget"); err != nil {
		return fmt.Errorf("non se atopou winget: instala MiKTeX e Pandoc á man, ou despois desde Lúa (Opcións)")
	}
	for _, id := range paquetesLua {
		// Comprobar antes con "winget list": se xa está, "winget install" devolve un
		// código de erro aínda que non haxa nada mal (caso real de MiKTeX).
		if comandoOculto("winget", "list", "--id", id, "-e").Run() == nil {
			log("  ✓ " + id)
			continue
		}
		log("  📦 " + id + " (winget)…")
		if err := executarConRexistro(comandoOculto("winget", "install", "--id", id, "-e", "--silent",
			"--accept-package-agreements", "--accept-source-agreements", "--scope", "machine"), log); err != nil {
			return fmt.Errorf("%s: %w", id, err)
		}
	}
	return nil
}

func desinstalarDepsLua(log func(string)) error {
	for _, id := range paquetesLua {
		log("  📦 " + id + " (winget uninstall)…")
		if err := executarConRexistro(comandoOculto("winget", "uninstall", "--id", id, "-e", "--silent"), log); err != nil {
			log("  ⚠️ " + id + ": " + err.Error())
		}
	}
	return nil
}

// ── Utilidades ───────────────────────────────────────────────────────────────

// comandoOculto é exec.Command sen abrir xanela de consola: o instalador é unha app
// gráfica e cada netsh/icacls/winget abriría unha.
func comandoOculto(nome string, args ...string) *exec.Cmd {
	cmd := exec.Command(nome, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000} // CREATE_NO_WINDOW
	return cmd
}

// O manifesto xa pide administrador (UAC) antes de main().
func asegurarPrivilexios()              {}
func dirConfigUsuario() (string, error) { return os.UserConfigDir() }
func axustarPropietario(string)         {}

// ── Ganchos que só usa macOS ─────────────────────────────────────────────────

// O instalador xa corre como administrador: a tarefa faise aquí mesmo.
func comoAdministrador(tr *Translator, t tarefaAdmin, log func(string)) (map[string]any, error) {
	return executarTarefa(tr, t, log)
}

func prepararApp(string, string) error     { return nil } // un .exe solto non leva bundle
func raizApp(dir string) string            { return dir }
func configFBSenContrasinal(fbRutas) error { return nil } // a ACL xa vale como está
