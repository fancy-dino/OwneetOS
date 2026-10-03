# iso/

The **archiso profile** that produces the OwneetOS bootable ISO.

- Based on Arch Linux's `baseline` profile, kept minimal.
- Arch packages are pinned to an **Arch Linux Archive snapshot date**; it moves forward only after testing.
- Our own packages come from the `[owneet]` repository built in [`packages/`](../packages/).
- The ISO must stay **under 2 GB** (GitHub Releases limit).
- Built only inside the builder VM, never on the host.

Roadmap: steps 1.1, 1.3, 1.4, 1.5, 1.6.
