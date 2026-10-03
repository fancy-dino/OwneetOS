# daemon/

**`owneetd`**, the OwneetOS system daemon. It holds the console together:

- reads controllers (evdev, SDL_GameControllerDB mappings) and owns the **Guide button**;
- provides a virtual keyboard and mouse (uinput) for the on-screen keyboard;
- manages Wi-Fi (NetworkManager), Bluetooth with automatic gamepad pairing (BlueZ), audio
  (PipeWire) and power;
- launches, tracks and closes games and apps;
- exposes a **local-only API** used by the frontend and the installer.

Language (Go or Rust) is decided in roadmap step 2.1.

Roadmap: phase 2.
