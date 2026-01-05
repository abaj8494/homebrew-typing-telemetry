class TypingTelemetry < Formula
  desc "Keystroke and mouse telemetry for developers - track your daily typing and mouse movement"
  homepage "https://github.com/abaj8494/typing-telemetry"
  version "0.7.0"
  license "MIT"

  # Install from GitHub repository
  url "https://github.com/abaj8494/typing-telemetry.git", tag: "v0.7.0"
  head "https://github.com/abaj8494/typing-telemetry.git", branch: "main"

  depends_on :macos
  depends_on "go" => :build

  def install
    system "go", "mod", "download"

    ldflags = "-s -w -X main.Version=#{version}"

    # Build CLI (no CGO required)
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel", "./cmd/typtel"

    # Build menu bar app (requires CGO for macOS frameworks)
    ENV["CGO_ENABLED"] = "1"
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel-menubar", "./cmd/typtel-menubar"

    # Build daemon (requires CGO for macOS frameworks)
    system "go", "build", *std_go_args(ldflags: ldflags), "-o", bin/"typtel-daemon", "./cmd/daemon"

    # Create .app bundle for easier accessibility permissions
    app_contents = prefix/"Typtel.app/Contents"
    app_contents.mkpath
    (app_contents/"MacOS").mkpath
    (app_contents/"Resources").mkpath

    # Copy binary into app bundle
    cp bin/"typtel-menubar", app_contents/"MacOS/typtel-menubar"

    # Create Info.plist
    (app_contents/"Info.plist").write <<~XML
      <?xml version="1.0" encoding="UTF-8"?>
      <!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
      <plist version="1.0">
      <dict>
          <key>CFBundleExecutable</key>
          <string>typtel-menubar</string>
          <key>CFBundleIdentifier</key>
          <string>com.typtel.menubar</string>
          <key>CFBundleName</key>
          <string>Typtel</string>
          <key>CFBundleDisplayName</key>
          <string>Typtel</string>
          <key>CFBundlePackageType</key>
          <string>APPL</string>
          <key>CFBundleVersion</key>
          <string>#{version}</string>
          <key>CFBundleShortVersionString</key>
          <string>#{version}</string>
          <key>LSMinimumSystemVersion</key>
          <string>10.13</string>
          <key>LSUIElement</key>
          <true/>
          <key>NSHighResolutionCapable</key>
          <true/>
          <key>NSHumanReadableCopyright</key>
          <string>Copyright 2024 Aayush Bajaj. MIT License.</string>
      </dict>
      </plist>
    XML
  end

  # Use Homebrew's service block for LaunchAgent management
  # Runs the binary inside the app bundle for consistent accessibility permissions
  service do
    run [opt_prefix/"Typtel.app/Contents/MacOS/typtel-menubar"]
    keep_alive true
    process_type :interactive
    log_path var/"log/typtel-menubar.log"
    error_log_path var/"log/typtel-menubar.log"
    environment_variables HOME: Dir.home
  end

  def post_install
    # Symlink app to ~/Applications for Finder/Spotlight access
    user_apps = Pathname.new(Dir.home)/"Applications"
    user_apps.mkpath
    target = user_apps/"Typtel.app"
    target.unlink if target.symlink? || target.exist?
    target.make_symlink(opt_prefix/"Typtel.app")
  end

  def caveats
    <<~EOS
      Typtel has been installed!

      TO START:
        brew services start typing-telemetry

      TO STOP:
        brew services stop typing-telemetry

      ACCESSIBILITY PERMISSIONS (one-time setup):
        1. Open System Settings > Privacy & Security > Accessibility
        2. Click + and navigate to ~/Applications/Typtel.app
           (or press Cmd+Shift+G and paste: ~/Applications/Typtel.app)
        3. Enable the checkbox for Typtel
        4. Restart: brew services restart typing-telemetry

      The app is symlinked to ~/Applications/Typtel.app for easy access.
      You can also find it via Spotlight (Cmd+Space, type "Typtel").

      COMMANDS:
        typtel           - Interactive dashboard
        typtel stats     - Show statistics
        typtel today     - Today's keystroke count
        typtel test      - Typing speed test

      FEATURES:
        - Keystroke and word counting
        - Mouse click tracking
        - Mouse movement tracking (in feet)
        - Configurable menu bar display (Settings menu)
        - Stillness Leaderboard
        - Charts and heatmaps

      The menu bar shows keystrokes and words by default.
      Use Settings to also show mouse clicks and distance.
    EOS
  end

  test do
    system "#{bin}/typtel", "today"
  end
end
