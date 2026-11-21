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
- `plataforma_linux.go`: Debian — servizo systemd (`piztu-filebrowser`, conta `www-data`), ufw, `.desktop`, apt.
- `plataforma_macos.go`: macOS — LaunchDaemon de launchd (`gal.omeu.piztu.filebrowser`, conta `_www`), firewall de aplicacións, bundles `.app`, Homebrew.
- `tarefas.go`: cada botón coma unha tarefa, para que macOS poida facela nun proceso con permisos de administrador.
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

## Debian

Instala o mesmo en `/opt/piztu/{filebrowser,modulos/tao,modulos/lua}`, config en
`/etc/filebrowser/config.yaml`, datos en `/var/lib/filebrowser`, arquivos en
`/var/www/html/arquivos` e rexistro en `/var/log/filebrowser.log`. File Browser corre como
servizo systemd con `www-data` (porto 80 mediante `CAP_NET_BIND_SERVICE`, se está libre;
se non, 8080). Se `ufw` está activo ábrese o porto. Lúa pode instalar Pandoc e TeX Live con apt.

O instalador necesita root: se se abre coma usuario normal relánzase con `pkexec`
(tamén vale `sudo ./instalador-aula`).

```bash
sudo apt install build-essential pkg-config libgtk-3-dev libwebkit2gtk-4.1-dev
./build-linux.sh            # descarga e verifica binarios, tests, compila (tag gtk3)
```

Saída: `build/bin/instalador-aula` e o seu `.sha256`. O SHA-256 do binario Linux de
Quantum está fixado en `build-linux.sh`.

## macOS

Tao e Lúa instálanse coma `/Applications/Tao.app` e `/Applications/Lua.app` (o mesmo
sitio que o instalador propio de Lúa para Mac, que queda substituído); aparecen en
Launchpad e Spotlight. File Browser queda en `/Library/Application Support/Piztu/FileBrowser`
(programa, `config.yaml` e `datos/`), os arquivos en `/Users/Shared/Piztu/arquivos` e o
rexistro en `/Library/Logs/Piztu/filebrowser.log`. Corre coma LaunchDaemon coa conta
`_www`: desde macOS 10.14 calquera proceso pode escoitar no porto 80. Se o firewall de
macOS está activo, engádese File Browser á súa lista de aplicacións permitidas. Lúa pode
instalar con Homebrew MacTeX (`mactex-no-gui`), Pandoc e Poppler; se non hai Homebrew,
instálao.

O instalador ábrese cun dobre clic e **non** se executa con sudo: Homebrew négase a
funcionar coma root. Os pasos que precisan root (File Browser sempre; Tao e Lúa só se
`/Applications` non se pode escribir) piden o contrasinal dun administrador co diálogo do
sistema e fanse nun proceso aparte (o propio instalador con `--piztu-tarefa-admin`).

```bash
xcode-select --install     # compilador de C (cgo), lipo e codesign
./build-macos.sh           # descarga e verifica binarios, tests, compila universal
```

Saída: `build/bin/instalador-aula.app` (universal, Apple Silicon e Intel, asinado ad hoc)
e `build/bin/instalador-aula-macos_<versión>.zip` co seu `.sha256`. Os SHA-256 dos
binarios de Quantum para macOS están fixados en `build-macos.sh`; o universal asínase ad
hoc porque a release só trae asinado o de arm64.

Coma non está notarizado, a primeira vez que se abre un instalador descargado macOS
bloquéao: clic dereito → Abrir, ou Axustes do Sistema → Privacidade e seguridade →
«Abrir igualmente».
