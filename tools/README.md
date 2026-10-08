# tools/

**Developer scripts**: wrappers that run builds inside the builder VM, release helpers, checks.

Scripts here must never modify the host system; anything that would requires the owner's approval.

Roadmap: introduced from step 0.7 onwards.

## Frontend preview

`tools/frontend-preview` shows the interface without booting an ISO: after
`tools/build-in-vm tools/build-packages owneet-frontend`, run
`tools/build-in-vm tools/frontend-preview tools/preview-steps/navigation.txt`. It starts the
frontend on a virtual screen in the builder VM with test games and a virtual controller, follows
the steps (keys, controller buttons, sticks, screenshots) and saves the screenshots in
`out/frontend-preview/`. The step format is described at the top of the script.
