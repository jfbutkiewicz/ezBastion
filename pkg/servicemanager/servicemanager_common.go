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

// Cmd is a command to send to the service.
// Analog to golang.org/x/sys/windows/svc.Cmd
// servicemanager/
// ├── servicemanager_common.go   # Cmd, State, MainService
// ├── servicemanager_windows.go  # SCM + eventlog
// ├── servicemanager_darwin.go   # launchd + plist
// └── servicemanager_linux.go    # systemd + unit

type Cmd uint32

const (
	Stop Cmd = iota
	Shutdown
	Start
)

// State is the target state attended.
// Analog to golang.org/x/sys/windows/svc.State
type State uint32

const (
	Stopped State = iota
	Running
	Starting
	Stopping
)

type MainService interface {
	StartMainService(serverchan *chan bool)
}
