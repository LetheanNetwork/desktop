package main

import (
	"runtime"

	core "dappco.re/go"
	command "dappco.re/go/process/exec"
)

func main() {
	core.Println("Checking Android development dependencies...")
	core.Println()

	errs := []string{}

	// Check Go
	if !checkCommand("go", "version") {
		errs = append(errs, "Go is not installed. Install from https://go.dev/dl/")
	} else {
		core.Println("✓ Go is installed")
	}

	// Check ANDROID_HOME
	androidHome := core.Getenv("ANDROID_HOME")
	if androidHome == "" {
		androidHome = core.Getenv("ANDROID_SDK_ROOT")
	}
	if androidHome == "" {
		// Try common default locations
		home := ""
		if r := core.UserHomeDir(); r.OK {
			home, _ = r.Value.(string)
		}
		possiblePaths := []string{
			core.PathJoin(home, "Android", "Sdk"),
			core.PathJoin(home, "Library", "Android", "sdk"),
			"/usr/local/share/android-sdk",
		}
		for _, p := range possiblePaths {
			if core.Stat(p).OK {
				androidHome = p
				break
			}
		}
	}

	if androidHome == "" {
		errs = append(errs, "ANDROID_HOME not set. Install Android Studio and set ANDROID_HOME environment variable")
	} else {
		core.Print(core.Stdout(), "✓ ANDROID_HOME: %s", androidHome)
	}

	// Check adb
	if !checkCommand("adb", "version") {
		if androidHome != "" {
			platformTools := core.PathJoin(androidHome, "platform-tools")
			errs = append(errs, core.Sprintf("adb not found. Add %s to PATH", platformTools))
		} else {
			errs = append(errs, "adb not found. Install Android SDK Platform-Tools")
		}
	} else {
		core.Println("✓ adb is installed")
	}

	// Check emulator
	if !checkCommand("emulator", "-list-avds") {
		if androidHome != "" {
			emulatorPath := core.PathJoin(androidHome, "emulator")
			errs = append(errs, core.Sprintf("emulator not found. Add %s to PATH", emulatorPath))
		} else {
			errs = append(errs, "emulator not found. Install Android Emulator via SDK Manager")
		}
	} else {
		core.Println("✓ Android Emulator is installed")
	}

	// Check NDK
	ndkHome := core.Getenv("ANDROID_NDK_HOME")
	if ndkHome == "" && androidHome != "" {
		// Look for NDK in default location
		ndkDir := core.PathJoin(androidHome, "ndk")
		if listing := core.ReadDir(core.DirFS(ndkDir), "."); listing.OK {
			entries, _ := listing.Value.([]core.FsDirEntry)
			for _, entry := range entries {
				if entry.IsDir() {
					ndkHome = core.PathJoin(ndkDir, entry.Name())
					break
				}
			}
		}
	}

	if ndkHome == "" {
		errs = append(errs, "Android NDK not found. Install NDK via Android Studio > SDK Manager > SDK Tools > NDK (Side by side)")
	} else {
		core.Print(core.Stdout(), "✓ Android NDK: %s", ndkHome)
	}

	// Check Java
	if !checkCommand("java", "-version") {
		errs = append(errs, "Java not found. Install JDK 11+ (OpenJDK recommended)")
	} else {
		core.Println("✓ Java is installed")
	}

	// Check for AVD (Android Virtual Device)
	if checkCommand("emulator", "-list-avds") {
		result := command.Command(core.Background(), "emulator", "-list-avds").Output()
		if result.OK && len(core.Trim(string(result.Value.([]byte)))) > 0 {
			avds := core.Split(core.Trim(string(result.Value.([]byte))), "\n")
			core.Print(core.Stdout(), "✓ Found %d Android Virtual Device(s)", len(avds))
		} else {
			core.Println("⚠ No Android Virtual Devices found. Create one via Android Studio > Tools > Device Manager")
		}
	}

	core.Println()

	if len(errs) > 0 {
		core.Println("❌ Missing dependencies:")
		for _, err := range errs {
			core.Print(core.Stdout(), "   - %s", err)
		}
		core.Println()
		core.Println("Setup instructions:")
		core.Println("1. Install Android Studio: https://developer.android.com/studio")
		core.Println("2. Open SDK Manager and install:")
		core.Println("   - Android SDK Platform (API 34)")
		core.Println("   - Android SDK Build-Tools")
		core.Println("   - Android SDK Platform-Tools")
		core.Println("   - Android Emulator")
		core.Println("   - NDK (Side by side)")
		core.Println("3. Set environment variables:")
		if runtime.GOOS == "darwin" {
			core.Println("   export ANDROID_HOME=$HOME/Library/Android/sdk")
		} else {
			core.Println("   export ANDROID_HOME=$HOME/Android/Sdk")
		}
		core.Println("   export PATH=$PATH:$ANDROID_HOME/platform-tools:$ANDROID_HOME/emulator")
		core.Println("4. Create an AVD via Android Studio > Tools > Device Manager")
		core.Exit(1)
	}

	core.Println("✓ All Android development dependencies are installed!")
}

func checkCommand(name string, args ...string) bool {
	return command.Command(core.Background(), name, args...).Run().OK
}
