# Application artwork

`sdrive.png` is the transparent master artwork. `sdrive.ico` contains 32-bit Windows icon frames at 16, 24, 32, 48, 64, 128 and 256 pixels. The application, diagnostic executable, installers and uninstaller share this icon. The tray retains its independent status indicators.

The master artwork was created with the built-in Imagegen tool using this prompt:

> Create one polished Windows desktop application icon for SDrive, a remote drive mounting app. A simple bold green storage drive base with a blue cloud above it, forming one cohesive recognizable symbol. Flat vector-like style with clean rounded shapes, restrained subtle shading, no text, no letters, no border tile, no background, genuinely transparent background. Centered square composition with generous small safe margins, strong silhouette, thick shapes that stay readable at 16 and 32 pixels. Green #21b993 and blue #378de5 with dark navy details. Single icon only, no presentation mockup or multiple variants.

## Documentation screenshot

`screenshots/overview.png` shows the actual React interface in English with fictional connection data, rendered at a 1440 by 900 viewport in the light theme. It does not use personal profiles or live provider accounts and is not evidence of provider compatibility. Refresh it when the interface materially changes, rather than for every release. Keep the README image path independent of the package version.
