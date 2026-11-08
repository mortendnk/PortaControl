// Command portacontrol-fakeac runs a simulated Midea PortaSplit on the LAN
// (TCP 6444, discovery UDP 6445/20086) for development without hardware.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
)

import "github.com/mortendnk/portacontrol/server/internal/fakeac"

func main() {
	addr := flag.String("listen", "0.0.0.0:6444", "TCP address of the device protocol")
	disc := flag.String("discovery", "0.0.0.0:6445", "UDP address for discovery (empty = off)")
	id := flag.Uint64("id", 151732605161920, "device id")
	flag.Parse()

	ac := fakeac.New(*id)
	if err := ac.Listen(*addr, *disc); err != nil {
		fmt.Fprintln(os.Stderr, "fakeac:", err)
		os.Exit(1)
	}

	fmt.Printf("simulated PortaSplit running\n  id    %d\n  port  %d\n  token %s\n  key   %s\n", ac.DeviceID, ac.Port, ac.TokenHex(), ac.KeyHex())

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, os.Interrupt)
	<-sig
	ac.Close()
}
