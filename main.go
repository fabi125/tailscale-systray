package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os/exec"
	"sync"
	"time"

	"github.com/gen2brain/beeep"
	"github.com/getlantern/systray"
)

var (
	//go:embed icon/on.png
	iconOn []byte
	//go:embed icon/off.png
	iconOff []byte
)

var (
	mu   sync.RWMutex
	myIP string
)

func main() {
	systray.Run(onReady, nil)
}

func executable(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

func doConnect(m *systray.MenuItem) {
	for {
		if _, ok := <-m.ClickedCh; !ok {
			break
		}

		cmd := exec.Command("sudo", "tailscale", "up", "--accept-routes", "--shields-up", "--json")
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			beeep.Notify(
				"Tailscale",
				err.Error(),
				"",
			)
			continue
		}
		if err := cmd.Start(); err != nil {
			beeep.Notify(
				"Tailscale",
				err.Error(),
				"",
			)
			continue
		}
		var upResult struct {
			AuthURL      string `json:"AuthURL"`
			BackendState string `json:"BackendState"`
		}
		if err := json.NewDecoder(stdout).Decode(&upResult); err != nil {
			beeep.Notify(
				"Tailscale",
				err.Error(),
				"",
			)
			continue
		}
		if upResult.BackendState == "NeedsLogin" {
			openBrowser(upResult.AuthURL)
		}
		if err := cmd.Wait(); err != nil {
			beeep.Notify(
				"Tailscale",
				err.Error(),
				"",
			)
			continue
		}
	}
}

func doDisconnect(m *systray.MenuItem) {
	for {
		if _, ok := <-m.ClickedCh; !ok {
			break
		}

		cmd := exec.Command("sudo", "tailscale", "down")
		if stdoutStderr, err := cmd.CombinedOutput(); err != nil {
			beeep.Notify(
				"Tailscale",
				string(stdoutStderr),
				"",
			)
		}
	}
}

func onReady() {
	systray.SetIcon(iconOff)

	mThisDevice := systray.AddMenuItem("", "")
	//mThisDevice.Disable()
	mStatus := systray.AddMenuItem("Status:", "")
	//mStatus.Disable()

	systray.AddSeparator()

	mConnect := systray.AddMenuItem("Connect", "")
	mConnect.Enable()
	mDisconnect := systray.AddMenuItem("Disconnect", "")
	mDisconnect.Disable()

	// if executable("pkexec") {
	go doConnect(mConnect)
	go doDisconnect(mDisconnect)
	// } else {
	//	mConnect.Hide()
	//	mDisconnect.Hide()s
	//}

	systray.AddSeparator()

	/*
		go func(mThisDevice *systray.MenuItem) {
			for {
				_, ok := <-mThisDevice.ClickedCh
				if !ok {
					break
				}
				mu.RLock()
				if myIP == "" {
					mu.RUnlock()
					continue
				}
				err := clipboard.WriteAll(myIP)
				if err == nil {
					beeep.Notify(
						"This device",
						fmt.Sprintf("Copy the IP address (%s) to the Clipboard", myIP),
						"",
					)
				}
				mu.RUnlock()
			}
		}(mThisDevice)

		mNetworkDevices := systray.AddMenuItem("Network Devices", "")
		mMyDevices := mNetworkDevices.AddSubMenuItem("My Devices", "")
		mTailscaleServices := mNetworkDevices.AddSubMenuItem("Tailscale Services", "")

		systray.AddSeparator()
		mAdminConsole := systray.AddMenuItem("Admin Console...", "")
		go func() {
			for {
				_, ok := <-mAdminConsole.ClickedCh
				if !ok {
					break
				}
				openBrowser("https://login.tailscale.com/admin/machines")
			}
		}()
	*/

	systray.AddSeparator()

	mExit := systray.AddMenuItem("Exit", "")
	go func() {
		<-mExit.ClickedCh
		systray.Quit()
	}()

	go func() {
		type Item struct {
			menu  *systray.MenuItem
			title string
			ip    string
			found bool
		}
		items := map[string]*Item{}

		enabled := false
		setDisconnected := func() {
			if enabled {
				systray.SetTooltip("Tailscale: Disconnected")
				mConnect.Enable()
				mDisconnect.Disable()
				systray.SetIcon(iconOff)
				enabled = false
			}
		}

		for {
			rawStatus, err := exec.Command("tailscale", "status", "--json").Output()
			if err != nil {
				setDisconnected()
				continue
			}

			status := new(Status)
			if err := json.Unmarshal(rawStatus, status); err != nil {
				setDisconnected()
				continue
			}

			mu.Lock()
			if len(status.Self.TailscaleIPs) != 0 {
				myIP = status.Self.TailscaleIPs[0]
			}
			mu.Unlock()

			if status.TailscaleUp && !enabled {
				systray.SetTooltip("Tailscale: Connected")
				mConnect.Disable()
				mDisconnect.Enable()
				systray.SetIcon(iconOn)
				enabled = true
			} else if !status.TailscaleUp && enabled {
				setDisconnected()
			}

			for _, v := range items {
				v.found = false
			}

			var statusStr string
			if status.TailscaleUp {
				statusStr = "Connected"
			} else {
				statusStr = "Disconnected"
			}
			keyLeft := status.Self.KeyExpiry.Sub(time.Now())
			var keyLeftStr string
			if keyLeft > 0 {
				keyLeftStr = fmt.Sprintf("key expires in %s", keyLeft.Round(time.Second))
			} else {
				keyLeftStr = "key expired"
			}
			mStatus.SetTitle(fmt.Sprintf("%s (%s)", statusStr, keyLeftStr))

			mThisDevice.SetTitle(fmt.Sprintf("%s (%s)", status.Self.DisplayName.String(), myIP))

			/*
				for _, peer := range status.Peers {
					ip := peer.TailscaleIPs[0]
					peerName := peer.DisplayName
					title := peerName.String()

					var sub *systray.MenuItem
					switch peerName.(type) {
					case DNSName:
						sub = mMyDevices
					case HostName:
						sub = mTailscaleServices
					}

					if item, ok := items[title]; ok {
						item.found = true
					} else {
						items[title] = &Item{
							menu:  sub.AddSubMenuItem(title, title),
							title: title,
							ip:    ip,
							found: true,
						}
						go func(item *Item) {
							// TODO fix race condition
							for {
								_, ok := <-item.menu.ClickedCh
								if !ok {
									break
								}
								err := clipboard.WriteAll(item.ip)
								if err != nil {
									beeep.Notify(
										"Tailscale",
										err.Error(),
										"",
									)
									return
								}
								beeep.Notify(
									item.title,
									fmt.Sprintf("Copy the IP address (%s) to the Clipboard", item.ip),
									"",
								)
							}
						}(items[title])
					}
				}

				for k, v := range items {
					if !v.found {
						// TODO fix race condition
						v.menu.Hide()
						delete(items, k)
					}
				}
			*/

			time.Sleep(10 * time.Second)
		}
	}()
}

func openBrowser(url string) {
	if err := exec.Command("xdg-open", url).Start(); err != nil {
		beeep.Notify(
			"Tailscale",
			"could not open link: "+err.Error(),
			"",
		)
	}
}
