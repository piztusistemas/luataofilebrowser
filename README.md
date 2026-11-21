# Instalador de aula

Instala no equipo do profesorado as tres pezas da aula:

| Peza | Que fai | Onde queda (Windows) |
|------|---------|----------------------|
| **File Browser Quantum** v1.5.6-stable | Servidor de arquivos da aula, como servizo `PiztuFileBrowser` (conta LocalService). Porto 80 se está libre, se non 8080, baixo `/arquivos`. Abre o porto no firewall en todos os perfís. | `%ProgramFiles%\Piztu\FileBrowser`, datos en `%ProgramData%\Piztu\FileBrowser`, arquivos en `%ProgramData%\Piztu\arquivos` |
| **Tao** | Prácticas e informes asinados; xestiona as contas do alumnado. Queda apuntando ao File Browser local (`tao-servidor.conf`). | `%ProgramFiles%\Piztu\Tao` |
| **Lúa** | Documentación e problemas (LaTeX/Word/ODT). Opcionalmente MiKTeX e Pandoc con winget. | `%ProgramFiles%\Piztu\Lua` (o mesmo ca o seu instalador propio) |

Non hai Apache: en Windows non hai portos privilexiados e Quantum escoita el mesmo no 80.
O alumnado (Windows ou Linux) descarga Tao das Releases e Tao atopa o servidor probando
o nome do equipo e os portos 80 e 8080.

## Contrasinal do admin de File Browser

O mesmo ciclo que o playbook de Piztu (`playbooks/instalar_filebrowser.yaml`):
contrasinal en `config.yaml` só para o primeiro arranque → login de proba → quítase do
ficheiro → reinicio → login de novo. Se Tao xa estaba configurado, actualízase o seu
`admin.json` para que non quede co contrasinal vello (bloquearía a conta admin).

## Estrutura

- `filebrowser.go`, `instalar.go`: lóxica común a todos os sistemas.
- `plataforma_windows.go`: servizo, ACL, firewall, accesos directos, winget.
- `plataforma_outras.go`: sitio para Debian e macOS (aínda sen facer).
- `cmd/fbservizo`: servizo de Windows que lanza Quantum (Quantum non fala co SCM).

## Compilar (Windows)

Precisa Go, Node, `wails3` e `task` (en `%USERPROFILE%\go\bin` vale).

```powershell
.\build-windows.ps1                       # descarga e verifica binarios, tests, compila
.\build-windows.ps1 -TaoVersion 26.11.6   # outra versión de Tao
```

Saída: `build\bin\instalador-aula.exe` e o seu `.sha256`.

Proba contra Quantum real (sen instalar nada no sistema):

```powershell
$env:FB_BIN="files\filebrowser.exe"; go test -run Quantum -v
```
