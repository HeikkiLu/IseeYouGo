package main

import (
	"flag"
	"fmt"
)

func main() {
	useCLI := flag.Bool("cli", false, "Launch with command line interface")
	showHelp := flag.Bool("help", false, "Show help message")

	flag.Parse()

	if *showHelp {
		fmt.Println("IseeYouGo - Laptop Lid Monitor")
		fmt.Println("Records video when your MacBook lid opens.")
		fmt.Println("")
		fmt.Println("Usage:")
		fmt.Println("  iseeyougo [options]")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -gui     Launch with graphical user interface")
		fmt.Println("  -cli     Launch with command line interface")
		fmt.Println("  -help    Show this help message")
		fmt.Println("")
		fmt.Println("If no option is specified, GUI mode is used by default.")
		return
	}

	if *useCLI {
		fmt.Println("Starting IseeYouGo in CLI mode...")
		runCLI()
	} else {
		fmt.Println("Starting IseeYouGo in GUI mode...")
		gui := NewGUI()
		gui.Run()
	}
}
