//go:build linux
// +build linux

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

// unitTemplate est le template du fichier systemd unit.
// Équivalent au plist launchd macOS / SCM Windows.
// Restart=always correspond à KeepAlive=true (launchd) / DelayedAutoStart (Windows).
const unitTemplate = `[Unit]
Description={{.Desc}}
After=network.target

[Service]
Type=simple
ExecStart={{.ExePath}}
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier={{.Name}}

[Install]
WantedBy=multi-user.target
`

// unitPath retourne le chemin du fichier unit systemd pour un service donné.
// Équivalent à plistPath (macOS) / registre SCM (Windows).
func unitPath(name string) string {
	return filepath.Join("/etc/systemd/system", name+".service")
}

// systemctl exécute une commande systemctl et retourne une erreur si elle échoue.
func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err.Error(), strings.TrimSpace(string(out)))
	}
	return nil
}

// StartService démarre le service systemd ciblé par name.
// Équivalent à StartService Windows (mgr.Connect + s.Start)
// et StartService macOS (launchctl load).
func StartService(name string) error {
	if err := systemctl("start", name); err != nil {
		log.Errorln(fmt.Sprintf("could not start service %s, error : %s", name, err.Error()))
		return fmt.Errorf("could not start service: %v", err)
	}
	return nil
}

// ControlService envoie une commande au service systemd ciblé par name.
// Équivalent à ControlService Windows (svc.Stop, svc.Shutdown...)
// et ControlService macOS (launchctl unload/load).
// c : commande à envoyer (Stop, Shutdown, Start)
// to : état cible attendu (Stopped, Running...)
func ControlService(name string, c Cmd, to State) error {
	var action string
	switch c {
	case Stop, Shutdown:
		action = "stop"
	case Start:
		action = "start"
	default:
		return fmt.Errorf("unsupported command: %d", c)
	}

	if err := systemctl(action, name); err != nil {
		log.Errorln(fmt.Sprintf("could not send control=%d to service %s: %s", c, name, err.Error()))
		return fmt.Errorf("could not send control=%d: %v", c, err)
	}

	// Vérifier l'état après la commande
	status, err := getServiceState(name)
	if err != nil {
		log.Warnln(fmt.Sprintf("could not verify service state: %s", err.Error()))
		return nil
	}
	if status != to {
		log.Warnln(fmt.Sprintf("service %s reached state %d, expected %d", name, status, to))
	}
	return nil
}

// InstallService installe le service systemd en créant le fichier unit.
// Équivalent à InstallService Windows (mgr.CreateService + eventlog.InstallAsEventCreate)
// et InstallService macOS (création du plist launchd).
func InstallService(name, desc, exePath string) error {
	exeFullPath, err := setupmanager.ExeFullPath()
	if err != nil {
		log.Errorln(err.Error())
		return err
	}

	unit := unitPath(name)
	if _, err := os.Stat(unit); err == nil {
		errormsg := fmt.Sprintf("service %s already exists", name)
		log.Errorln(errormsg)
		return fmt.Errorf(errormsg)
	}

	type unitData struct {
		Name    string
		ExePath string
		Desc    string
	}

	_ = exePath // exePath fourni en paramètre, on utilise exeFullPath détecté
	data := unitData{Name: name, ExePath: exeFullPath, Desc: desc}

	tmpl, err := template.New("unit").Parse(unitTemplate)
	if err != nil {
		log.Errorln(err.Error())
		return err
	}

	f, err := os.Create(unit)
	if err != nil {
		log.Errorln(fmt.Sprintf("could not create unit file %s: %s", unit, err.Error()))
		return err
	}
	defer f.Close()

	if err = tmpl.Execute(f, data); err != nil {
		log.Errorln(err.Error())
		return err
	}

	// Permissions du fichier unit (root:root, 644) — requis par systemd
	if err = os.Chown(unit, 0, 0); err != nil {
		log.Warnln(fmt.Sprintf("could not chown unit %s: %s (may need sudo)", unit, err.Error()))
	}
	if err = os.Chmod(unit, 0644); err != nil {
		log.Warnln(fmt.Sprintf("could not chmod unit %s: %s", unit, err.Error()))
	}

	// Recharger systemd pour prendre en compte le nouveau fichier unit
	if err = systemctl("daemon-reload"); err != nil {
		log.Errorln(fmt.Sprintf("daemon-reload failed: %s", err.Error()))
		return err
	}

	// Activer le service au démarrage (équivalent DelayedAutoStart / RunAtLoad)
	if err = systemctl("enable", name); err != nil {
		log.Errorln(fmt.Sprintf("could not enable service %s: %s", name, err.Error()))
		return err
	}

	log.Infoln(fmt.Sprintf("service %s installed at %s", name, unit))
	return nil
}

// RemoveService supprime le service systemd ciblé par name.
// Équivalent à RemoveService Windows (s.Delete + eventlog.Remove)
// et RemoveService macOS (launchctl unload + os.Remove).
func RemoveService(name string) error {
	unit := unitPath(name)

	if _, err := os.Stat(unit); os.IsNotExist(err) {
		errormsg := fmt.Sprintf("service %s is not installed", name)
		log.Errorln(errormsg)
		return fmt.Errorf(errormsg)
	}

	// Arrêter et désactiver le service avant suppression
	_ = systemctl("stop", name)
	_ = systemctl("disable", name)

	if err = os.Remove(unit); err != nil {
		log.Errorln(err.Error())
		return err
	}

	// Recharger systemd après suppression
	if err = systemctl("daemon-reload"); err != nil {
		log.Warnln(fmt.Sprintf("daemon-reload failed after removal: %s", err.Error()))
	}

	log.Infoln(fmt.Sprintf("service %s removed", name))
	return nil
}

// getServiceState retourne l'état courant du service via systemctl is-active.
// Équivalent à getServiceState macOS (launchctl list).
func getServiceState(name string) (State, error) {
	out, err := exec.Command("systemctl", "is-active", name).Output()
	if err != nil {
		return Stopped, nil // service inactif
	}
	switch strings.TrimSpace(string(out)) {
	case "active":
		return Running, nil
	case "activating":
		return Starting, nil
	case "deactivating":
		return Stopping, nil
	default:
		return Stopped, nil
	}
}

// RunService lance le service en mode daemon ou debug.
// Équivalent à RunService Windows (svc.Run / debug.Run + eventlog)
// et RunService macOS (os/signal + logrus).
// En mode debug, les logs vont sur stdout via logrus.
// En mode daemon, systemd capture stdout/stderr via journald (cf. unit template).
func RunService(name string, isDebug bool, MS MainService) {
	ms = MS

	if isDebug {
		log.SetOutput(os.Stdout)
		log.Infoln(fmt.Sprintf("starting %s service (debug mode)", name))
	} else {
		// En production, systemd redirige stdout vers journald (StandardOutput=journal)
		// On peut aussi écrire dans un fichier explicite si besoin
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
	// systemd envoie SIGTERM pour arrêter, SIGINT en mode debug
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	go ms.StartMainService(&serverchan)

	// Attente du signal d'arrêt
	sig := <-sigChan
	log.Infoln(fmt.Sprintf("%s service received signal: %s", name, sig.String()))
	close(serverchan)

	log.Infoln(fmt.Sprintf("%s service stopped", name))
}
