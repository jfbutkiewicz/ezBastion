//go:build darwin
// +build darwin

// This file is part of ezBastion.

//     ezBastion is free software: you can redistribute it and/or modify
//     it under the terms of the GNU Affero General Public License as published by
//     the Free Software Foundation, either version 3 of the License, or
//     (at your option) any later version.

//     ezBastion is distributed in the hope that it will be useful,
//     but WITHOUT ANY WARRANTY; without even the implied warranty of
//     MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
//     GNU Affero General Public License for more details.

//     You should have received a copy of the GNU Affero General Public License
//     along with ezBastion.  If not, see <https://www.gnu.org/licenses/>.

package servicemanager

import (
	"ezBastion/pkg/setupmanager"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"text/template"

	log "github.com/sirupsen/logrus"
)

var (
	err error
	ms  MainService
)

type MyService struct{}

// plistTemplate est le template du fichier launchd plist pour macOS.
// Il correspond à la configuration du service Windows (DelayedAutoStart, StartType auto).
const plistTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>{{.Name}}</string>

    <key>ProgramArguments</key>
    <array>
        <string>{{.ExePath}}</string>
    </array>

    <key>RunAtLoad</key>
    <true/>

    <key>KeepAlive</key>
    <true/>

    <key>StandardOutPath</key>
    <string>/var/log/{{.Name}}.log</string>

    <key>StandardErrorPath</key>
    <string>/var/log/{{.Name}}.err</string>

    <key>Comment</key>
    <string>{{.Desc}}</string>
</dict>
</plist>
`

// plistPath retourne le chemin du fichier plist launchd pour un service donné.
// Équivalent à l'entrée dans le registre SCM Windows.
func plistPath(name string) string {
	return filepath.Join("/Library/LaunchDaemons", name+".plist")
}

// StartService démarre le daemon launchd ciblé par name.
// Équivalent à StartService Windows (mgr.Connect + s.Start).
func StartService(name string) error {
	if err := exec.Command("launchctl", "load", "-w", plistPath(name)).Run(); err != nil {
		log.Errorln(fmt.Sprintf("could not start service %s, error : %s", name, err.Error()))
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

// ControlService envoie une commande au daemon launchd ciblé par name.
// Équivalent à ControlService Windows (svc.Stop, svc.Shutdown...).
// c : commande à envoyer (Stop, Shutdown, Start)
// to : état cible attendu (Stopped, Running...)
func ControlService(name string, c Cmd, to State) error {
	var lctlCmd string
	switch c {
	case Stop, Shutdown:
		lctlCmd = "unload"
	case Start:
		lctlCmd = "load"
	default:
		return fmt.Errorf("unsupported command: %d", c)
	}

	if err := exec.Command("launchctl", lctlCmd, "-w", plistPath(name)).Run(); err != nil {
		log.Errorln(fmt.Sprintf("could not send control=%d to service %s: %s", c, name, err.Error()))
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}

	// Vérifier l'état après la commande (équivalent du polling Windows)
	status, err := getServiceState(name)
	if err != nil {
		log.Warnln(fmt.Sprintf("could not verify service state: %s", err.Error()))
		return nil // commande envoyée, état non vérifiable
	}
	if status != to {
		log.Warnln(fmt.Sprintf("service %s reached state %d, expected %d", name, status, to))
	}
	return nil
}

// getServiceState retourne l'état courant du service via launchctl list.
func getServiceState(name string) (State, error) {
	out, err := exec.Command("launchctl", "list", name).Output()
	if err != nil {
		return Stopped, nil // service non chargé = arrêté
	}
	if strings.Contains(string(out), `"PID"`) {
		return Running, nil
	}
	return Stopped, nil
}

// InstallService installe le daemon launchd en créant le fichier plist.
// Équivalent à InstallService Windows (mgr.CreateService + eventlog.InstallAsEventCreate).
func InstallService(name, desc, exePath string) error {
	exeFullPath, err := setupmanager.ExeFullPath()
	if err != nil {
		log.Errorln(err.Error())
		return err
	}

	plist := plistPath(name)
	if _, err := os.Stat(plist); err == nil {
		errormsg := fmt.Sprintf("service %s already exists", name)
		log.Errorln(errormsg)
		return fmt.Errorf(errormsg)
	}

	type plistData struct {
		Name    string
		ExePath string
		Desc    string
	}

	_ = exePath // exePath fourni en paramètre, on utilise exeFullPath détecté
	data := plistData{Name: name, ExePath: exeFullPath, Desc: desc}

	tmpl, err := template.New("plist").Parse(plistTemplate)
	if err != nil {
		log.Errorln(err.Error())
		return err
	}

	f, err := os.Create(plist)
	if err != nil {
		log.Errorln(fmt.Sprintf("could not create plist file %s: %s", plist, err.Error()))
		return err
	}
	defer f.Close()

	if err = tmpl.Execute(f, data); err != nil {
		log.Errorln(err.Error())
		return err
	}

	// Fixer les permissions du plist (root:wheel, 644) — requis par launchd
	if err = os.Chown(plist, 0, 0); err != nil {
		log.Warnln(fmt.Sprintf("could not chown plist %s: %s (may need sudo)", plist, err.Error()))
	}
	if err = os.Chmod(plist, 0644); err != nil {
		log.Warnln(fmt.Sprintf("could not chmod plist %s: %s", plist, err.Error()))
	}

	log.Infoln(fmt.Sprintf("service %s installed at %s", name, plist))
	return nil
}

// RemoveService supprime le daemon launchd ciblé par name.
// Équivalent à RemoveService Windows (s.Delete + eventlog.Remove).
func RemoveService(name string) error {
	plist := plistPath(name)

	if _, err := os.Stat(plist); os.IsNotExist(err) {
		errormsg := fmt.Sprintf("service %s is not installed", name)
		log.Errorln(errormsg)
		return fmt.Errorf(errormsg)
	}

	// Décharger le service avant suppression (ignore l'erreur si déjà arrêté)
	_ = exec.Command("launchctl", "unload", "-w", plist).Run()

	if err = os.Remove(plist); err != nil {
		log.Errorln(err.Error())
		return err
	}

	log.Infoln(fmt.Sprintf("service %s removed", name))
	return nil
}

// RunService lance le service en mode daemon ou debug.
// Équivalent à RunService Windows (svc.Run / debug.Run + eventlog).
// En mode debug, les logs vont sur stdout via logrus.
// En mode daemon, le process écoute SIGTERM/SIGINT pour s'arrêter proprement.
func RunService(name string, isDebug bool, MS MainService) {
	ms = MS

	if isDebug {
		log.SetOutput(os.Stdout)
		log.Infoln(fmt.Sprintf("starting %s service (debug mode)", name))
	} else {
		// En production, logrus écrit dans le fichier de log défini dans le plist
		logFile, err := os.OpenFile(
			fmt.Sprintf("/var/log/%s.log", name),
			os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644,
		)
		if err != nil {
			log.Warnln(fmt.Sprintf("could not open log file, falling back to stdout: %s", err.Error()))
		} else {
			log.SetOutput(logFile)
			defer logFile.Close()
		}
		log.Infoln(fmt.Sprintf("starting %s service", name))
	}

	serverchan := make(chan bool)

	// Écoute des signaux système (équivalent svc.Stop / svc.Shutdown)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go ms.StartMainService(&serverchan)

	// Attente du signal d'arrêt
	sig := <-sigChan
	log.Infoln(fmt.Sprintf("%s service received signal: %s", name, sig.String()))
	close(serverchan)

	log.Infoln(fmt.Sprintf("%s service stopped", name))
}
