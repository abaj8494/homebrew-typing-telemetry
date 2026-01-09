cask "typtel" do
  version "1.1.7"
  sha256 "7a2627504c7a0c765f71c2233b4309ebe45ce2f1078edd8a15009828ed61be17"

  url "https://github.com/abaj8494/homebrew-typing-telemetry/releases/download/v#{version}/Typtel-#{version}.zip"
  name "Typtel"
  desc "Keystroke and mouse distance metrics for developers"
  homepage "https://github.com/abaj8494/typing-telemetry"

  app "Typtel.app"

  postflight do
    system_command "/usr/bin/xattr",
                   args: ["-cr", "#{appdir}/Typtel.app"],
                   sudo: false
  end

  binary "Typtel.app/Contents/MacOS/typtel"

  uninstall launchctl: "com.typtel.menubar"

  zap trash: [
    "~/.local/share/typtel",
    "~/Library/LaunchAgents/com.typtel.menubar.plist",
  ]

  caveats <<~EOS
    Typtel requires Accessibility permissions to track keystrokes.

    SETUP:
      1. Open System Settings > Privacy & Security > Accessibility
      2. Click + and select /Applications/Typtel.app
      3. Enable the checkbox

    AFTER UPGRADING:
      macOS requires re-granting permissions when the binary changes.
      If Typtel won't launch, remove it from Accessibility and re-add it.

    START:
      Open Typtel from Spotlight (Cmd+Space, type "Typtel")
      Or run: open /Applications/Typtel.app

    The app will appear in your menu bar.

    COMMANDS:
      typtel           - Interactive dashboard
      typtel stats     - Show statistics
      typtel today     - Today's keystroke count
      typtel test      - Typing speed test
      typtel v         - View charts in browser

    To start automatically at login, enable "Launch at Login" in the menu bar.
  EOS
end
