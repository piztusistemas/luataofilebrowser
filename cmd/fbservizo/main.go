//go:build windows

// fbservizo é o servizo de Windows de File Browser Quantum. Quantum é un programa de
// consola que non fala co xestor de servizos (SCM): rexistrado tal cal, Windows dáo por
// morto aos 30 s (erro 1053). Este envoltorio si fala co SCM, lanza filebrowser.exe,
// pasa a súa saída a un ficheiro de rexistro e detéño cando Windows o pide. Se Quantum
// cae, o servizo remata con erro e as accións de recuperación (instalarServizoFB)
// vólveno arrincar.
//
// Uso (ponlle os argumentos o instalador):
//
//	fbservizo.exe -fb <filebrowser.exe> -c <config.yaml> -dir <cartafol de traballo> -log <ficheiro>
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"golang.org/x/sys/windows/svc"
)

const nomeServizo = "PiztuFileBrowser"

// tamañoMaxRexistro: ao arrincar, se o rexistro pasa disto, gárdase coma .1 e
// empézase outro (unha soa copia: abonda para ver o último problema).
const tamañoMaxRexistro = 10 << 20

type servizo struct {
	fb, config, dir, rexistro string
}

func main() {
	s := &servizo{}
	flag.StringVar(&s.fb, "fb", "", "ruta de filebrowser.exe")
	flag.StringVar(&s.config, "c", "", "config.yaml de Quantum")
	flag.StringVar(&s.dir, "dir", "", "cartafol de traballo")
	flag.StringVar(&s.rexistro, "log", "", "ficheiro de rexistro")
	flag.Parse()

	enServizo, err := svc.IsWindowsService()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if !enServizo {
		// Execución a man (probas): o mesmo, en primeiro plano.
		cmd, err := s.lanzar(os.Stdout)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		cmd.Wait()
		return
	}
	if err := svc.Run(nomeServizo, s); err != nil {
		os.Exit(1)
	}
}

func (s *servizo) abrirRexistro() io.Writer {
	if s.rexistro == "" {
		return io.Discard
	}
	if st, err := os.Stat(s.rexistro); err == nil && st.Size() > tamañoMaxRexistro {
		os.Rename(s.rexistro, s.rexistro+".1")
	}
	f, err := os.OpenFile(s.rexistro, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return io.Discard
	}
	return f
}

func (s *servizo) lanzar(saida io.Writer) (*exec.Cmd, error) {
	cmd := exec.Command(s.fb, "-c", s.config)
	cmd.Dir = s.dir
	cmd.Stdout, cmd.Stderr = saida, saida
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("non se puido lanzar %s: %w", s.fb, err)
	}
	return cmd, nil
}

// Execute implementa svc.Handler.
func (s *servizo) Execute(_ []string, peticions <-chan svc.ChangeRequest, estado chan<- svc.Status) (bool, uint32) {
	estado <- svc.Status{State: svc.StartPending}

	rex := s.abrirRexistro()
	fmt.Fprintf(rex, "\n== %s fbservizo: arrincando %s -c %s\n", time.Now().Format(time.RFC3339), s.fb, s.config)
	cmd, err := s.lanzar(rex)
	if err != nil {
		fmt.Fprintln(rex, err)
		return true, 1
	}
	rematou := make(chan error, 1)
	go func() { rematou <- cmd.Wait() }()

	estado <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-rematou:
			// Quantum caeu por si só: saír con erro para que actúe a recuperación.
			fmt.Fprintf(rex, "== %s filebrowser rematou: %v\n", time.Now().Format(time.RFC3339), err)
			return true, 2
		case p := <-peticions:
			switch p.Cmd {
			case svc.Interrogate:
				estado <- p.CurrentStatus
			case svc.Stop, svc.Shutdown:
				estado <- svc.Status{State: svc.StopPending}
				// Windows non ten SIGTERM para procesos de consola sen xanela: Kill.
				// Quantum escribe na base de datos (bbolt) con transaccións, así que
				// un final brusco non a deixa corrupta.
				cmd.Process.Kill()
				select {
				case <-rematou:
				case <-time.After(10 * time.Second):
				}
				fmt.Fprintf(rex, "== %s fbservizo: detido\n", time.Now().Format(time.RFC3339))
				return false, 0
			}
		}
	}
}
