// install_deps.go - iOS development dependency checker
// This script checks for required iOS development tools.
// It's designed to be portable across different shells by using Go instead of shell scripts.
//
// Usage:
//   go run install_deps.go                      # Interactive mode
//   TASK_FORCE_YES=true go run install_deps.go  # Auto-accept prompts
//   CI=true go run install_deps.go              # CI mode (auto-accept)

package main

import (
	"bufio"

	core "dappco.re/go"
	command "dappco.re/go/process/exec"
)

type Dependency struct {
	Name       string
	CheckFunc  func() (bool, string) // Returns (success, details)
	Required   bool
	InstallCmd []string
	InstallMsg string
	SuccessMsg string
	FailureMsg string
}

func main() {
	core.Println("Checking iOS development dependencies...")
	core.Println("===================================================")
	core.Println()

	hasErrors := false
	dependencies := []Dependency{
		{
			Name: "Xcode",
			CheckFunc: func() (bool, string) {
				// Check if xcodebuild exists
				if !checkCommand([]string{"xcodebuild", "-version"}) {
					return false, ""
				}
				// Get version info
				out, ok := commandOutput("xcodebuild", "-version")
				if !ok {
					return false, ""
				}
				lines := core.Split(string(out), "\n")
				if len(lines) > 0 {
					return true, core.Trim(lines[0])
				}
				return true, ""
			},
			Required:   true,
			InstallMsg: "Please install Xcode from the Mac App Store:\n   https://apps.apple.com/app/xcode/id497799835\n   Xcode is REQUIRED for iOS development (includes iOS SDKs, simulators, and frameworks)",
			SuccessMsg: "✅ Xcode found",
			FailureMsg: "❌ Xcode not found (REQUIRED)",
		},
		{
			Name: "Xcode Developer Path",
			CheckFunc: func() (bool, string) {
				// Check if xcode-select points to a valid Xcode path
				out, ok := commandOutput("xcode-select", "-p")
				if !ok {
					return false, "xcode-select not configured"
				}
				path := core.Trim(string(out))

				// Check if path exists and is in Xcode.app
				if !core.Stat(path).OK {
					return false, "Invalid Xcode path"
				}

				// Verify it's pointing to Xcode.app (not just Command Line Tools)
				if !core.Contains(path, "Xcode.app") {
					return false, core.Sprintf("Points to %s (should be Xcode.app)", path)
				}

				return true, path
			},
			Required:   true,
			InstallCmd: []string{"sudo", "xcode-select", "-s", "/Applications/Xcode.app/Contents/Developer"},
			InstallMsg: "Xcode developer path needs to be configured",
			SuccessMsg: "✅ Xcode developer path configured",
			FailureMsg: "❌ Xcode developer path not configured correctly",
		},
		{
			Name: "iOS SDK",
			CheckFunc: func() (bool, string) {
				// Get the iOS Simulator SDK path
				output, ok := commandOutput("xcrun", "--sdk", "iphonesimulator", "--show-sdk-path")
				if !ok {
					return false, "Cannot find iOS SDK"
				}
				sdkPath := core.Trim(string(output))

				// Check if the SDK path exists
				if !core.Stat(sdkPath).OK {
					return false, "iOS SDK path not found"
				}

				// Check for UIKit framework (essential for iOS development)
				uikitPath := core.Sprintf("%s/System/Library/Frameworks/UIKit.framework", sdkPath)
				if !core.Stat(uikitPath).OK {
					return false, "UIKit.framework not found"
				}

				// Get SDK version
				versionOut, _ := commandOutput("xcrun", "--sdk", "iphonesimulator", "--show-sdk-version")
				version := core.Trim(string(versionOut))

				return true, core.Sprintf("iOS %s SDK", version)
			},
			Required:   true,
			InstallMsg: "iOS SDK comes with Xcode. Please ensure Xcode is properly installed.",
			SuccessMsg: "✅ iOS SDK found with UIKit framework",
			FailureMsg: "❌ iOS SDK not found or incomplete",
		},
		{
			Name: "iOS Simulator Runtime",
			CheckFunc: func() (bool, string) {
				if !checkCommand([]string{"xcrun", "simctl", "help"}) {
					return false, ""
				}
				// Check if we can list runtimes
				out, ok := commandOutput("xcrun", "simctl", "list", "runtimes")
				if !ok {
					return false, "Cannot access simulator"
				}
				// Count iOS runtimes
				lines := core.Split(string(out), "\n")
				count := 0
				var versions []string
				for _, line := range lines {
					if core.Contains(line, "iOS") && !core.Contains(line, "unavailable") {
						count++
						// Extract version number
						if parts := fields(line); len(parts) > 2 {
							for _, part := range parts {
								if core.HasPrefix(part, "(") && core.HasSuffix(part, ")") {
									versions = append(versions, core.TrimCutset(part, "()"))
									break
								}
							}
						}
					}
				}
				if count > 0 {
					return true, core.Sprintf("%d runtime(s): %s", count, core.Join(", ", versions...))
				}
				return false, "No iOS runtimes installed"
			},
			Required:   true,
			InstallMsg: "iOS Simulator runtimes come with Xcode. You may need to download them:\n   Xcode → Settings → Platforms → iOS",
			SuccessMsg: "✅ iOS Simulator runtime available",
			FailureMsg: "❌ iOS Simulator runtime not available",
		},
	}

	// Check each dependency
	for _, dep := range dependencies {
		success, details := dep.CheckFunc()
		if success {
			msg := dep.SuccessMsg
			if details != "" {
				msg = core.Sprintf("%s (%s)", dep.SuccessMsg, details)
			}
			core.Println(msg)
		} else {
			core.Println(dep.FailureMsg)
			if details != "" {
				core.Print(core.Stdout(), "   Details: %s", details)
			}
			if dep.Required {
				hasErrors = true
				if len(dep.InstallCmd) > 0 {
					core.Println()
					core.Println("   " + dep.InstallMsg)
					core.Print(core.Stdout(), "   Fix command: %s", core.Join(" ", dep.InstallCmd...))
					if promptUser("Do you want to run this command?") {
						core.Println("Running command...")
						if r := runInteractive(dep.InstallCmd); !r.OK {
							core.Print(core.Stdout(), "Command failed: %v", r.Value)
							core.Exit(1)
						}
						core.Println("✅ Command completed. Please run this check again.")
					} else {
						core.Print(core.Stdout(), "   Please run manually: %s", core.Join(" ", dep.InstallCmd...))
					}
				} else {
					core.Println("   " + dep.InstallMsg)
				}
			}
		}
	}

	// Check for iPhone simulators
	core.Println()
	core.Println("Checking for iPhone simulator devices...")
	if !checkCommand([]string{"xcrun", "simctl", "list", "devices"}) {
		core.Println("❌ Cannot check for iPhone simulators")
		hasErrors = true
	} else {
		out, ok := commandOutput("xcrun", "simctl", "list", "devices")
		if !ok {
			core.Println("❌ Failed to list simulator devices")
			hasErrors = true
		} else if !core.Contains(string(out), "iPhone") {
			core.Println("⚠️  No iPhone simulator devices found")
			core.Println()

			// Get the latest iOS runtime
			runtimeOut, ok := commandOutput("xcrun", "simctl", "list", "runtimes")
			if !ok {
				core.Println("   Failed to get iOS runtimes")
			} else {
				lines := core.Split(string(runtimeOut), "\n")
				var latestRuntime string
				for _, line := range lines {
					if core.Contains(line, "iOS") && !core.Contains(line, "unavailable") {
						// Extract runtime identifier
						parts := fields(line)
						if len(parts) > 0 {
							latestRuntime = parts[len(parts)-1]
						}
					}
				}

				if latestRuntime == "" {
					core.Println("   No iOS runtime found. Please install iOS simulators in Xcode:")
					core.Println("   Xcode → Settings → Platforms → iOS")
				} else {
					core.Println("   Would you like to create an iPhone 15 Pro simulator?")
					createCmd := []string{"xcrun", "simctl", "create", "iPhone 15 Pro", "iPhone 15 Pro", latestRuntime}
					core.Print(core.Stdout(), "   Command: %s", core.Join(" ", createCmd...))
					if promptUser("Create simulator?") {
						if r := runInteractive(createCmd); !r.OK {
							core.Print(core.Stdout(), "   Failed to create simulator: %v", r.Value)
						} else {
							core.Println("   ✅ iPhone 15 Pro simulator created")
						}
					} else {
						core.Println("   Skipping simulator creation")
						core.Print(core.Stdout(), "   Create manually: %s", core.Join(" ", createCmd...))
					}
				}
			}
		} else {
			// Count iPhone devices
			count := 0
			lines := core.Split(string(out), "\n")
			for _, line := range lines {
				if core.Contains(line, "iPhone") && !core.Contains(line, "unavailable") {
					count++
				}
			}
			core.Print(core.Stdout(), "✅ %d iPhone simulator device(s) available", count)
		}
	}

	// Final summary
	core.Println()
	core.Println("===================================================")
	if hasErrors {
		core.Println("❌ Some required dependencies are missing or misconfigured.")
		core.Println()
		core.Println("Quick setup guide:")
		core.Println("1. Install Xcode from Mac App Store (if not installed)")
		core.Println("2. Open Xcode once and agree to the license")
		core.Println("3. Install additional components when prompted")
		core.Println("4. Run: sudo xcode-select -s /Applications/Xcode.app/Contents/Developer")
		core.Println("5. Download iOS simulators: Xcode → Settings → Platforms → iOS")
		core.Println("6. Run this check again")
		core.Exit(1)
	} else {
		core.Println("✅ All required dependencies are installed!")
		core.Println("   You're ready for iOS development with Wails!")
	}
}

func checkCommand(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return command.Command(core.Background(), args[0], args[1:]...).Run().OK
}

func promptUser(question string) bool {
	// Check if we're in a non-interactive environment
	if core.Getenv("CI") != "" || core.Getenv("TASK_FORCE_YES") == "true" {
		core.Print(core.Stdout(), "%s [y/N]: y (auto-accepted)", question)
		return true
	}

	reader := bufio.NewReader(core.Stdin())
	core.Print(core.Stdout(), "%s [y/N]: ", question)

	response, err := reader.ReadString('\n')
	if err != nil {
		return false
	}

	response = core.Lower(core.Trim(response))
	return response == "y" || response == "yes"
}

func commandOutput(name string, args ...string) ([]byte, bool) {
	r := command.Command(core.Background(), name, args...).Output()
	if !r.OK {
		return nil, false
	}
	out, _ := r.Value.([]byte)
	return out, true
}

func runInteractive(args []string) core.Result {
	if len(args) == 0 {
		return core.Fail(core.E("install_deps.runInteractive", "empty command", nil))
	}
	return command.Command(core.Background(), args[0], args[1:]...).
		WithStdout(core.Stdout()).
		WithStderr(core.Stderr()).
		WithStdin(core.Stdin()).
		Run()
}

func fields(line string) []string {
	raw := core.Split(core.Trim(line), " ")
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = core.Trim(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
