# Setting up your workspace

OwneetOS lives in **two repositories**: this one, with the code, and the **knowledge base**, an
[Obsidian](https://obsidian.md) vault with every requirement, decision, plan and design document.
The knowledge base is the source of truth: you need both. This guide gets both onto your computer,
in one workspace folder, ready for building OwneetOS and for working with an AI assistant
(Claude Code).

| Repository | What it is | Link |
|---|---|---|
| `OwneetOS` | The code (this repository) | <https://github.com/fancy-dino/OwneetOS> |
| `OwneetOS-knowledge-base` | The knowledge base (Obsidian vault) | <https://github.com/Danidepadawan/OwneetOS-knowledge-base> |

Both repositories are public.

## 1. Minimal requirements

### To build and run OwneetOS

ISOs and packages are built in an Arch Linux **builder VM** and booted in a **test VM**, both under
QEMU/KVM. Nothing is ever built on your own system (see `vm/README.md`).

- **A PC running Linux on x86_64**, with CPU virtualisation (Intel VT-x / AMD-V) turned on in the
  firmware, and `/dev/kvm` readable and writable by your user. macOS and Windows cannot run the VMs.
- **Memory and CPUs:** the builder VM uses 8 GB of RAM and 8 CPUs, the test VM 4 GB and 4 CPUs
  (13 GB to test Steam in the live session). To run both at once, plan on at least 16 GB of RAM.
- **Disk:** the builder's disk grows up to 80 GB; with ISOs (about 2 GB each) and test disks, keep
  about 100 GB free.
- **Packages:**

  | What | Debian / Ubuntu / Mint | Arch |
  |---|---|---|
  | QEMU | `qemu-system-x86`, `qemu-utils` | `qemu-base` |
  | UEFI firmware | `ovmf` | `edk2-ovmf` |
  | VM windows and 3D acceleration | `qemu-system-gui` | `qemu-ui-gtk`, `qemu-hw-display-virtio-gpu-gl` |
  | Tools used by the VM scripts | `git`, `curl`, `gnupg`, `openssh-client`, `rsync`, `python3` | `git`, `curl`, `gnupg`, `openssh`, `rsync`, `python` |

  Everything else (archiso, Go, Node.js, ffmpeg…) is installed inside the builder VM.

### To use the knowledge base

Any system (Linux, macOS, Windows):

- **Git** — <https://git-scm.com/downloads>
- **Obsidian** (desktop app, free) to read and edit the vault — <https://obsidian.md/download>
- **Python 3 — required** — <https://www.python.org/downloads/>. It runs the guard check before
  every tool call of Claude Code and rebuilds the vault's indexes. **Without it, Claude Code cannot
  use any tool in the workspace**: the guard check blocks every call (on purpose: the guards are
  never skipped) and says why. Linux distributions ship it; on macOS it comes with Apple's Command
  Line Tools (`xcode-select --install`).
- **Claude Code**, if you work with the AI assistant —
  <https://docs.anthropic.com/en/docs/claude-code/overview>. The workspace files below use a POSIX
  shell: Linux or macOS (on Windows, use WSL).

## 2. Workspace structure

Both repositories sit side by side in one workspace folder, next to two files that set up the AI
assistant:

```text
owneet-workspace/                  any name; Claude Code is started here
├── CLAUDE.md                      created by you (step 3.2)
├── .claude/
│   └── settings.json              created by you (step 3.2)
├── OwneetOS/                      this repository
└── OwneetOS-knowledge-base/       the knowledge base (opened in Obsidian)
```

- The two repository folders **must keep these names**: the workspace files and the knowledge
  base's scripts refer to them.
- The workspace folder is not a repository, so `CLAUDE.md` and `.claude/settings.json` are kept
  here, in this guide.

## 3. Steps

### 3.1 Clone both repositories

```text
mkdir owneet-workspace
cd owneet-workspace
git clone https://github.com/fancy-dino/OwneetOS.git
git clone https://github.com/Danidepadawan/OwneetOS-knowledge-base.git
```

### 3.2 Create the workspace files

`owneet-workspace/CLAUDE.md` loads the knowledge base's rules and its **guards** (what must never be
done) in every Claude Code session:

```markdown
# OwneetOS workspace

- `OwneetOS/` — the code repository (github.com/fancy-dino/OwneetOS).
- `OwneetOS-knowledge-base/` — the Obsidian vault with all project knowledge (rules, decisions,
  plans, designs). It is the source of truth; its guards are binding in every session.

@OwneetOS-knowledge-base/CLAUDE.md
```

`owneet-workspace/.claude/settings.json` makes the guards binding and keeps the vault's indexes
current. Every hook goes through the knowledge base's `scripts/hook.sh`:

- **before every tool call**, `guard-check.py` blocks what the guards forbid (pushing, GitHub
  actions, private keys, building on your PC) and asks you before anything is installed on your PC;
  if the check cannot run (no working Python), the call is blocked and the reason is shown;
- **with every prompt**, a one-line reminder that the guards are in force;
- **after every compaction** of the conversation, the guards are loaded again, word for word
  (plain shell: this and the reminder work even without Python);
- **when a session starts and after every note written or edited**, the `INDEX.md` files are
  rebuilt.

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "compact",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"$CLAUDE_PROJECT_DIR\"/OwneetOS-knowledge-base/scripts/hook.sh reinject",
            "statusMessage": "Re-loading OwneetOS guards"
          }
        ]
      },
      {
        "matcher": "startup|resume|clear",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"$CLAUDE_PROJECT_DIR\"/OwneetOS-knowledge-base/scripts/hook.sh indexes",
            "timeout": 30,
            "statusMessage": "Rebuilding OwneetOS vault indexes"
          }
        ]
      }
    ],
    "UserPromptSubmit": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "sh \"$CLAUDE_PROJECT_DIR\"/OwneetOS-knowledge-base/scripts/hook.sh remind",
            "timeout": 10
          }
        ]
      }
    ],
    "PreToolUse": [
      {
        "matcher": "Bash|Read|Edit|MultiEdit|Write|Grep|Glob|NotebookEdit",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"$CLAUDE_PROJECT_DIR\"/OwneetOS-knowledge-base/scripts/hook.sh guard-check",
            "timeout": 10,
            "onFailure": "block"
          }
        ]
      }
    ],
    "PostToolUse": [
      {
        "matcher": "Write|Edit",
        "hooks": [
          {
            "type": "command",
            "command": "sh \"$CLAUDE_PROJECT_DIR\"/OwneetOS-knowledge-base/scripts/hook.sh indexes",
            "timeout": 30
          }
        ]
      }
    ]
  }
}
```

### 3.3 Open the knowledge base in Obsidian

In Obsidian, **Open folder as vault** and choose `owneet-workspace/OwneetOS-knowledge-base`. Start
from `INDEX.md`: every folder has one, with one line per note. Notes are linked as
`[[note-name]]`, and the same links appear in this repository's code comments and documents.

The indexes are rebuilt by Claude Code, not by Obsidian: after adding or describing a note in
Obsidian, run `python3 scripts/build-indexes.py` in the vault (or let the next Claude Code session
do it).

### 3.4 Start Claude Code in the workspace folder

```text
cd owneet-workspace
claude
```

Start it in the workspace folder. In the session, `/memory` lists the knowledge base's `CLAUDE.md`
and `/hooks` lists the five hooks.

Starting it inside `OwneetOS/` works too: this repository's own `.claude/settings.json` runs the
same hooks through `.claude/kb-hook.sh` **when the knowledge base is next to it**
(`../OwneetOS-knowledge-base`); if the workspace's `CLAUDE.md` (step 3.2) is missing, the guards
and the vault's rules are loaded by the session-start hook instead. If the knowledge base is not
there, the session starts with a warning that the knowledge base is missing and that its guards
and hooks are not in use; nothing else changes.

### 3.5 Check that everything works

```text
python3 OwneetOS-knowledge-base/scripts/build-indexes.py --check   # knowledge base: no output
cd OwneetOS
vm/vm.sh selftest                                                  # KVM and UEFI firmware work
vm/builder.sh setup                                                # builder VM, once
tools/build-in-vm tools/lint                                       # the project's checks pass
```

Building and booting an ISO: `iso/README.md`. Everything about the VMs: `vm/README.md`.

## 4. Next

- How to contribute (commits, checks, licence headers): [`CONTRIBUTING.md`](../CONTRIBUTING.md).
- In the knowledge base: `INDEX.md`, then `kb-rules.md` (how the vault is kept) and the guards in
  `guards/`, which apply to everyone working on the project.
