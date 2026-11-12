-- SPDX-License-Identifier: GPL-3.0-or-later
--
-- OwneetOS's player ([[step-5.6-local-videos-and-music]], [[local-media]], approved demo
-- design/demos/5.6-videos-and-music.html): mpv, started by owneetd with this folder as its
-- configuration. This script gives the buttons their meanings ("Inside the player" in
-- [[controller-input-map]]; input.conf names them) and draws the player's display:
--   - a video: the folder and the title, the progress and the times, the Audio / Subtitles /
--     Autoplay states with their buttons, the prompts, and short messages at the top. It shows
--     for a moment at every press, and stays while paused; the subtitles move up meanwhile.
--   - a song: the "now playing" screen (its picture, title, artist, album, progress, the next song).
-- What it shows comes from the console, in the session file owneetd writes
-- (--script-opts=owneet-session=FILE): the texts already translated (the message files), the
-- palette's colours, the fonts, the controller's glyphs, the files with their titles. Without it
-- nothing is drawn (no colours or texts of its own). It writes no file: mpv saves a video's
-- position itself (mpv.conf).
-- It tells owneetd what plays ("owneet-state" messages on mpv's control socket) and ignores the
-- buttons while the console is on screen over it ("owneet-focus" from owneetd): mpv reads the
-- controller itself, through SDL, also while its window is not shown.

local mp = require "mp"
local utils = require "mp.utils"
local options = require "mp.options"

local opts = { session = "" }
options.read_options(opts, "owneet")

---------------------------------------------------------------------------------------------------
-- The session

local S = { kind = "videos", mode = "gamescope", autoplay = true, items = {}, texts = {},
            languages = {}, codes = {}, look = {} }
do
    local f = opts.session ~= "" and io.open(opts.session, "r")
    if f then
        local data = utils.parse_json(f:read("*a") or "")
        f:close()
        if type(data) == "table" then
            for k, v in pairs(data) do S[k] = v end
        end
    else
        mp.msg.warn("no session file: nothing is drawn")
    end
end
local video = S.kind ~= "music"
local look = S.look or {}
local drawn = type(look.bg) == "string"          -- the console's colours are there
local function T(key) return (S.texts or {})[key] or "" end
-- the placeholders ("{t}"…) are left in the texts by the console and filled here
local function fill(text, args)
    return (text:gsub("{(%w+)}", function(k) return args[k] ~= nil and tostring(args[k]) or "" end))
end
-- times as the console writes them: 3:34, 1:02:05
local function clock(s)
    s = math.max(0, math.floor(s or 0))
    local h, m = math.floor(s / 3600), math.floor(s / 60) % 60
    if h > 0 then return string.format("%d:%02d:%02d", h, m, s % 60) end
    return string.format("%d:%02d", m, s % 60)
end

---------------------------------------------------------------------------------------------------
-- What plays

local function index() return (mp.get_property_number("playlist-pos", 0) or 0) + 1 end
local function count() return mp.get_property_number("playlist-count", #S.items) or #S.items end
local function item(i) return S.items[i or index()] or {} end
local function position() return mp.get_property_number("time-pos", 0) or 0 end
local function duration() return mp.get_property_number("duration", 0) or 0 end
local function paused() return mp.get_property_bool("pause", false) end
local function tracks(kind)
    local out = {}
    for _, t in ipairs(mp.get_property_native("track-list") or {}) do
        if t.type == kind then out[#out + 1] = t end
    end
    return out
end
-- a track's language by the console's names (two-letter codes; Matroska and MP4 files have
-- three-letter ones), else its title, else "Track 2"
-- a track's language as a two-letter code where the console knows it ("ita" → "it")
local function code_of(t)
    local code = tostring(t.lang or ""):lower():gsub("[-_].*$", "")
    if #code == 3 then code = (S.codes or {})[code] or code end
    return code:match("^%a%a%a?$") and code or ""
end
local function language(t, i)
    local code = code_of(t)
    local name = code ~= "" and (S.languages or {})[code] or nil
    if name then return name end
    if t.title and t.title ~= "" then return t.title end
    return fill(T("player.track"), { i = i })
end
local function current_track(kind)
    local id = mp.get_property_number("current-tracks/" .. kind .. "/id")
    for i, t in ipairs(tracks(kind)) do
        if t.id == id then return t, i end
    end
    return nil, 0
end

local autoplay = S.autoplay ~= false
-- the console's preferred languages for videos (owner, 2026-11-12, [[local-media]]): the audio
-- track last chosen with X, the subtitles last chosen with Menu ("off" too); "" until a choice
local pref_audio, pref_subs = S.audio_language or "", S.subtitles or ""
local on_screen = true             -- owneetd says when the console is on screen over the player
local failed = ""                  -- the last file that could not be played
local picture = false              -- a song shows its picture (mpv draws it as the video)

-- owneetd follows what plays (the console's "Playing", its home screen's hero, Autoplay)
local function send_state()
    mp.commandv("script-message", "owneet-state", utils.format_json({
        index = index() - 1, path = mp.get_property("path") or (item().path or ""),
        position = position(), duration = duration(), paused = paused(),
        autoplay = autoplay, error = failed, audio_language = pref_audio, subtitles = pref_subs,
    }))
end

---------------------------------------------------------------------------------------------------
-- Drawing: ASS events on mpv's OSD, on the console's 1280 x 720 grid scaled to the screen

local RX, RY = 1280, 720
local TS = tonumber(look.textScale) or 1
local function fs(v) return v * TS end          -- text sizes follow the console's "Text size"
local TEXT, DISPLAY = look.textFont or "", look.displayFont or ""
-- libass sizes a font by its whole height (the OS/2 table's winAscent + winDescent): these are the
-- bundled fonts' (tools/branding/make-fonts), so that a size here is the console's pixel size
local EM = { ["Lexend"] = 1.59, ["Bricolage Grotesque"] = 1.56 }
local function font(family, px, weight)
    return string.format("\\fn%s\\fs%.3f\\b%d", family, px * (EM[family] or 1.25), weight or 400)
end

local function colour(c)            -- "#RRGGBB" → ASS's "&HBBGGRR&"
    c = type(c) == "string" and c:match("^#?(%x%x%x%x%x%x)") or "000000"
    return string.format("&H%s%s%s&", c:sub(5, 6), c:sub(3, 4), c:sub(1, 2))
end
local function alpha(opacity)
    return string.format("&H%02X&", math.floor((1 - math.max(0, math.min(1, opacity))) * 255 + 0.5))
end
local BG, SURFACE, LINE = colour(look.bg), colour(look.surface), colour(look.line)
local FG, MUTED, ACCENT = colour(look.fg), colour(look.muted), colour(look.accent)
-- generated art (songs without a picture) and the glow behind it: from a hue, as the console's
-- GameArt draws covers
local function hsl(h, s, l)
    h = (h % 360) / 360
    local q = l < 0.5 and l * (1 + s) or l + s - l * s
    local p = 2 * l - q
    local function part(t)
        t = t % 1
        if t < 1 / 6 then return p + (q - p) * 6 * t end
        if t < 1 / 2 then return q end
        if t < 2 / 3 then return p + (q - p) * (2 / 3 - t) * 6 end
        return p
    end
    local function b(v) return math.floor(v * 255 + 0.5) end
    return string.format("#%02X%02X%02X", b(part(h + 1 / 3)), b(part(h)), b(part(h - 1 / 3)))
end

local BASE = "\\bord0\\shad0\\blur0\\fsp0\\fscx100\\fscy100\\frz0\\q2"
local function escape(text)
    local ok, out = pcall(mp.command_native, { "escape-ass", tostring(text or "") })
    return ok and out or tostring(text or ""):gsub("[{}\\]", "")
end
local function text_ev(x, y, an, style, c, op, text, extra)
    return string.format("{\\an%d\\pos(%.2f,%.2f)%s%s\\1c%s\\1a%s%s}%s", an, x, y, BASE, style, c,
                         alpha(op or 1), extra or "", escape(text))
end

-- Shapes (\p4: coordinates × 8). libass places a drawing by its bounding box, so each shape is
-- written from its own top-left corner and put there with \pos.
local Path = {}
Path.__index = Path
local function path() return setmetatable({ cmds = {}, x0 = math.huge, y0 = math.huge }, Path) end
function Path:add(cmd, ...)
    local pts = { ... }
    for i = 1, #pts, 2 do
        self.x0, self.y0 = math.min(self.x0, pts[i]), math.min(self.y0, pts[i + 1])
    end
    self.cmds[#self.cmds + 1] = { cmd, pts }
    return self
end
function Path:m(x, y) return self:add("m", x, y) end
function Path:l(x, y) return self:add("l", x, y) end
function Path:b(...) return self:add("b", ...) end
local K = 0.5523                    -- a quarter circle as a Bézier curve
function Path:rrect(x, y, w, h, r, reverse)
    r = math.max(0, math.min(r, w / 2, h / 2))
    local c = r * K
    if not reverse then
        self:m(x + r, y):l(x + w - r, y):b(x + w - r + c, y, x + w, y + r - c, x + w, y + r)
            :l(x + w, y + h - r):b(x + w, y + h - r + c, x + w - r + c, y + h, x + w - r, y + h)
            :l(x + r, y + h):b(x + r - c, y + h, x, y + h - r + c, x, y + h - r)
            :l(x, y + r):b(x, y + r - c, x + r - c, y, x + r, y)
    else                            -- the other way round: a hole in the shape before
        self:m(x + r, y):b(x + r - c, y, x, y + r - c, x, y + r)
            :l(x, y + h - r):b(x, y + h - r + c, x + r - c, y + h, x + r, y + h)
            :l(x + w - r, y + h):b(x + w - r + c, y + h, x + w, y + h - r + c, x + w, y + h - r)
            :l(x + w, y + r):b(x + w, y + r - c, x + w - r + c, y, x + w - r, y)
    end
    return self
end
function Path:circle(cx, cy, r, reverse) return self:rrect(cx - r, cy - r, 2 * r, 2 * r, r, reverse) end
-- an open line, drawn there and back (no area) so that its border is the stroke
function Path:line(pts, ox, oy, s)
    local function at(i) return ox + pts[i] * s, oy + pts[i + 1] * s end
    self:m(at(1))
    for i = 3, #pts, 2 do self:l(at(i)) end
    for i = #pts - 3, 1, -2 do self:l(at(i)) end
    return self
end
function Path:coords(ox, oy)
    local out = {}
    for _, c in ipairs(self.cmds) do
        out[#out + 1] = c[1]
        for i = 1, #c[2], 2 do
            out[#out + 1] = math.floor((c[2][i] - ox) * 8 + 0.5) .. " " .. math.floor((c[2][i + 1] - oy) * 8 + 0.5)
        end
    end
    return table.concat(out, " ")
end
-- filled; with `stroke` (its width), only the line is drawn
function Path:ass(c, op, stroke, extra)
    local look_tags = stroke
        and string.format("\\1a&HFF&\\3c%s\\3a%s\\bord%.3f", c, alpha(op or 1), stroke / 2)
        or string.format("\\1c%s\\1a%s", c, alpha(op or 1))
    return string.format("{\\an7\\pos(%.3f,%.3f)%s%s%s\\p4}%s{\\p0}", self.x0, self.y0,
                         BASE, look_tags, extra or "", self:coords(self.x0, self.y0))
end
-- as a clip: the screen's own coordinates
function Path:clip(inverse) return string.format("\\%sclip(4,%s)", inverse and "i" or "", self:coords(0, 0)) end

-- Text widths, from libass itself (a hidden overlay), kept per text
local measurer, widths = nil, {}
local function width(style, text)
    local key = style .. "\n" .. text
    if widths[key] then return widths[key] end
    if not measurer then
        measurer = mp.create_osd_overlay("ass-events")
        measurer.hidden, measurer.compute_bounds = true, true
    end
    measurer.res_x, measurer.res_y = RX, RY
    measurer.data = string.format("{\\an7\\pos(0,0)%s%s}%s", BASE, style, escape(text))
    local r = measurer:update()
    local w = r and r.x0 and (r.x1 - r.x0) or nil
    if not w then                   -- no screen yet: a guess, not kept
        local px = tonumber(style:match("\\fs([%d.]+)")) or 20
        local n = 0
        for _ in text:gmatch("[%z\1-\127\194-\244][\128-\191]*") do n = n + 1 end
        return n * px * 0.36
    end
    widths[key] = w
    return w
end
-- cut to a width, with "…"
local function fit(style, text, max)
    text = tostring(text or "")
    if max <= 0 or width(style, text) <= max then return text end
    local chars = {}
    for ch in text:gmatch("[%z\1-\127\194-\244][\128-\191]*") do chars[#chars + 1] = ch end
    local lo, hi = 0, #chars
    while lo < hi do
        local mid = math.floor((lo + hi + 1) / 2)
        if width(style, table.concat(chars, "", 1, mid) .. "…") <= max then lo = mid else hi = mid - 1 end
    end
    return (table.concat(chars, "", 1, lo):gsub("%s+$", "")) .. "…"
end

-- The controller's buttons, as the console's Glyph draws them (a 26 × 26 grid): letters in a
-- circle (Xbox-style, Nintendo-style), the four shapes (PlayStation-style), pills for triggers
local PILLS = {
    xbox = { lt = { "LT", 32 }, rt = { "RT", 32 } },
    ps = { lt = { "L2", 30 }, rt = { "R2", 30 }, menu = { "OPTIONS", 62 } },
    nintendo = { lt = { "ZL", 32 }, rt = { "ZR", 32 } },
}
local LINES = {
    ["ps.a"] = { { 9.2, 9.2, 16.8, 16.8 }, { 16.8, 9.2, 9.2, 16.8 } },
    ["ps.x"] = { { 8.8, 8.8, 17.2, 8.8, 17.2, 17.2, 8.8, 17.2, 8.8, 8.8 } },
    ["ps.y"] = { { 13, 8.4, 17.8, 16.6, 8.2, 16.6, 13, 8.4 } },
    ["xbox.menu"] = { { 8.5, 10, 17.5, 10 }, { 8.5, 13, 17.5, 13 }, { 8.5, 16, 17.5, 16 } },
    ["nintendo.menu"] = { { 13, 8.5, 13, 17.5 }, { 8.5, 13, 17.5, 13 } },
    guide = { { 8, 13.2, 13, 8.8, 18, 13.2 }, { 9.6, 12, 9.6, 17.4, 16.4, 17.4, 16.4, 12 } },
    dpad = { { 11, 7.5, 15, 7.5, 15, 11, 18.5, 11, 18.5, 15, 15, 15, 15, 18.5, 11, 18.5, 11, 15,
               7.5, 15, 7.5, 11, 11, 11, 11, 7.5 } },
}
local SET = PILLS[look.glyphs] and look.glyphs or "xbox"
local function glyph_width(button, size) local p = PILLS[SET][button]; return (p and p[2] or 26) * size / 26 end
local function glyph(out, button, x, y, size, c)
    local s, p = size / 26, PILLS[SET][button]
    local w = glyph_width(button, size)
    if p then
        out[#out + 1] = path():rrect(x + s, y + s, w - 2 * s, 24 * s, 7 * s)
            :rrect(x + 2.8 * s, y + 2.8 * s, w - 5.6 * s, 20.4 * s, 5.2 * s, true):ass(c, 1)
        out[#out + 1] = text_ev(x + w / 2, y + 13.3 * s, 5, font(TEXT, 11.5 * s, 600), c, 1, p[1])
        return w
    end
    out[#out + 1] = path():circle(x + 13 * s, y + 13 * s, 12 * s):circle(x + 13 * s, y + 13 * s, 10.2 * s, true):ass(c, 1)
    if SET ~= "ps" and (button == "a" or button == "b" or button == "x" or button == "y") then
        out[#out + 1] = text_ev(x + 13 * s, y + 13.3 * s, 5, font(TEXT, 13 * s, 600), c, 1, button:upper())
    elseif SET == "ps" and button == "b" then
        out[#out + 1] = path():circle(x + 13 * s, y + 13 * s, 4.6 * s):circle(x + 13 * s, y + 13 * s, 2.6 * s, true):ass(c, 1)
    else
        for _, l in ipairs(LINES[SET .. "." .. button] or LINES[button] or {}) do
            out[#out + 1] = path():line(l, x, y, s):ass(c, 1, 2 * s)
        end
    end
    return w
end

-- The prompts at the bottom: what the buttons do (left), Back and Guide (right)
local LABEL = font(TEXT, fs(14.5), 400)
local function prompts(out, cy)
    local size = fs(26)
    local left = {
        { { "a" }, T(paused() and "player.play" or "player.pause") },
        { { "dpad" }, T("player.seek") },
        { { "lt", "rt" }, T("player.prevnext") },
    }
    local right = {
        { { "b" }, T("prompt.back") },
        -- in cage the console cannot be shown over the player: Guide held closes it
        { { "guide" }, T(S.mode == "cage" and "player.hold" or "prompt.home") },
    }
    local function wide(p)
        local w = 0
        for _, b in ipairs(p[1]) do w = w + glyph_width(b, size) + 8 end
        return w + 1 + width(LABEL, p[2])
    end
    local function draw(p, x)
        for _, b in ipairs(p[1]) do x = x + glyph(out, b, x, cy - size / 2, size, FG) + 8 end
        out[#out + 1] = text_ev(x + 1, cy, 4, LABEL, MUTED, 1, p[2])
        return x + 1 + width(LABEL, p[2])
    end
    local x = 64
    for _, p in ipairs(left) do x = draw(p, x) + 26 end
    local total = -26
    for _, p in ipairs(right) do total = total + wide(p) + 26 end
    x = RX - 64 - total
    for _, p in ipairs(right) do x = draw(p, x) + 26 end
end

-- The progress: the time, the bar, the length; the bar keeps its place while the time changes
local TIME = font(TEXT, fs(14.5), 400)
local function progress(out, x0, x1, cy)
    local pos, dur = position(), duration()
    local a, b = clock(pos), clock(dur)
    local slot = math.max(width(TIME, (a:gsub("%d", "0"))), width(TIME, (b:gsub("%d", "0"))))
    out[#out + 1] = text_ev(x0, cy, 4, TIME, FG, 1, a)
    out[#out + 1] = text_ev(x1, cy, 6, TIME, FG, 1, b)
    local bx0, bx1 = x0 + slot + 16, x1 - slot - 16
    if bx1 <= bx0 then return end
    out[#out + 1] = path():rrect(bx0, cy - 4, bx1 - bx0, 8, 4):ass(FG, 0.22)
    local f = dur > 0 and math.max(0, math.min(1, pos / dur)) or 0
    if f > 0 then out[#out + 1] = path():rrect(bx0, cy - 4, math.max(8, (bx1 - bx0) * f), 8, 4):ass(ACCENT, 1) end
end

-- Soft shades (the demo's gradients): copies one inside the other, each adding a little, so
-- that the whole reaches `want` inside copy k
local function step(want, before) return want >= 1 and 1 or 1 - (1 - want) / (1 - before) end

-- A message at the top ("Resumed at 12:04", "+10 s", "Audio: Italian (2 of 2)")
local message, message_until = "", 0
local MESSAGE = font(TEXT, fs(17), 400)
local function draw_message(out)
    local w = width(MESSAGE, message) + 44
    local h = fs(17) * 1.25 + 20
    local x = (RX - w) / 2
    out[#out + 1] = path():rrect(x, 44, w, h, 18):rrect(x + 1, 45, w - 2, h - 2, 17, true):ass(LINE, 1)
    out[#out + 1] = path():rrect(x + 1, 45, w - 2, h - 2, 17):ass(BG, 0.9)
    out[#out + 1] = text_ev(RX / 2, 44 + h / 2, 5, MESSAGE, FG, 1, message)
end

-- Paused: the sign in the middle (of the screen; of the picture for a song)
local function draw_pause(out, cx, cy)
    out[#out + 1] = path():circle(cx, cy, 52):ass(BG, 0.75)
    local s = 44 / 24
    for _, bx in ipairs({ 8.5, 15.5 }) do
        out[#out + 1] = path():line({ bx, 5.5, bx, 18.5 }, cx - 22, cy - 22, s):ass(FG, 1, 2 * s)
    end
end

-- A video's display: the folder and the title, the states, the progress, the prompts, over a
-- shade of the console's background from the bottom
local CRUMB, TITLE = font(TEXT, fs(14.5), 400), font(DISPLAY, fs(24), 700)
local STATE = font(TEXT, fs(14.5), 400)
local function draw_video(out)
    local top = RY - 270
    local before = 0
    for k = 0, 31 do                          -- the shade: 94 % at the bottom, 72 % half-way, then none
        local f = 1 - (k + 1) / 32            -- how far from the bottom this band starts
        local want = f <= 0.55 and (0.94 - 0.22 * f / 0.55) or 0.72 * (1 - (f - 0.55) / 0.45)
        local a = step(want, before)
        before = want
        out[#out + 1] = path():rrect(-20, top + 270 * k / 32, RX + 40, 270 * (32 - k) / 32 + 20, 0):ass(BG, a, nil, "\\blur6")
    end
    local size = fs(26)
    -- the states, on the right of the title: Audio (several tracks), Subtitles (any), Autoplay
    local states = {}
    local audio, subs = tracks("audio"), tracks("sub")
    if #audio > 1 then
        local t, i = current_track("audio")
        states[#states + 1] = { "x", fill(T("player.state.audio"), { l = t and language(t, i) or "" }) }
    end
    if #subs > 0 then
        local t, i = current_track("sub")
        states[#states + 1] = { "menu", fill(T("player.state.subtitles"), { l = t and language(t, i) or T("player.off") }) }
    end
    states[#states + 1] = { "y", fill(T("media.autoplay"), { state = T(autoplay and "media.on" or "media.off") }) }
    local x = RX - 64
    local bottom = RY - 96
    for k = #states, 1, -1 do
        local st = states[k]
        local w = 5 + glyph_width(st[1], size) + 8 + width(STATE, st[2]) + 13
        local h = size + 8
        x = x - w
        out[#out + 1] = path():rrect(x, bottom - h, w, h, h / 2):rrect(x + 1, bottom - h + 1, w - 2, h - 2, h / 2 - 1, true):ass(LINE, 1)
        out[#out + 1] = path():rrect(x + 1, bottom - h + 1, w - 2, h - 2, h / 2 - 1):ass(SURFACE, 0.85)
        glyph(out, st[1], x + 5, bottom - h + 4, size, FG)
        out[#out + 1] = text_ev(x + 5 + glyph_width(st[1], size) + 8, bottom - h / 2, 4, STATE, FG, 1, st[2])
        x = x - 10
    end
    local it = item()
    local room = x - 64 - 24
    out[#out + 1] = text_ev(64, bottom - fs(24) * 1.05 / 2, 4, TITLE, FG, 1, fit(TITLE, it.title or "", room))
    if (it.folder or "") ~= "" then
        out[#out + 1] = text_ev(64, bottom - fs(24) * 1.05 - 2 - fs(14.5) * 1.25 / 2, 4, CRUMB, MUTED, 1, fit(CRUMB, it.folder, room))
    end
    progress(out, 64, RX - 64, RY - 75)
    prompts(out, RY - 41)
end

-- A song's screen: its picture (mpv shows it as the video, in the box) or art from the album's
-- hue, beside the title, artist, album, progress and the next song; a glow of the hue behind
local LABEL_UP = font(TEXT, fs(12.5), 500)
local SONG_TITLE, ARTIST, ALBUM = font(DISPLAY, fs(40), 700), font(TEXT, fs(24), 400), font(TEXT, fs(17), 400)
local SMALL = font(TEXT, fs(14.5), 400)
local function art_box() local cy = (60 + RY - 150) / 2; return 120, cy - 170, 340, cy end
local function draw_song(out)
    local it = item()
    local hue = tonumber(it.hue) or 0
    local ax, ay, as, cy = art_box()
    local rounded = path():rrect(ax, ay, as, as, 20)
    -- the console's background around the picture (whatever mpv's output does with the rest of
    -- the window), the picture's corners rounded like the console's art
    out[#out + 1] = path():rrect(-10, -10, RX + 20, RY + 20, 0):ass(BG, 1, nil, rounded:clip(true))
    if not picture then
        out[#out + 1] = rounded:ass(colour(hsl(hue + 12, 0.47, 0.19)), 1)
        local clip = rounded:clip()
        local glow = { { 0.8, 0.1, 0.7, hsl(hue, 0.85, 0.62), 0.95 }, { 0, 1, 0.65, hsl(hue + 40, 0.70, 0.35), 1 } }
        for _, g in ipairs(glow) do
            local before = 0
            for k = 1, 18 do
                local want = g[5] * k / 18
                local a = step(want, before)
                before = want
                local r = as * g[3] * (1 - (k - 1) / 18)
                out[#out + 1] = path():circle(ax + as * g[1], ay + as * g[2], r):ass(colour(g[4]), a, nil, clip .. "\\blur8")
            end
        end
    end
    -- the glow of the album's hue, never over the picture
    do
        local before, c, gx, gy = 0, colour(hsl(hue, 0.60, 0.45)), RX * 0.22, RY * 0.35
        local clip = rounded:clip(true)
        for k = 1, 28 do
            local want = 0.45 * k / 28
            local a = step(want, before)
            before = want
            local f = 1 - (k - 1) / 28
            out[#out + 1] = path():rrect(gx - RX * 0.49 * f, gy - RY * 0.63 * f, 2 * RX * 0.49 * f, 2 * RY * 0.63 * f, RX)
                :ass(c, a, nil, clip .. "\\blur20")
        end
    end
    -- the words, beside the picture
    local x, room = ax + as + 60, RX - 120 - (ax + as + 60)
    local lines = {
        { 12.5 * 1.25, LABEL_UP, MUTED, T("label.playing"), "\\fsp" .. string.format("%.2f", fs(12.5) * 0.09) },
        { 40 * 1.05, SONG_TITLE, FG, it.title or "" },
        { 24 * 1.25, ARTIST, FG, (it.artist or "") ~= "" and it.artist or T("media.unknown_artist") },
        { 17 * 1.25, ALBUM, MUTED, it.album or "" },
        { 14.5 * 1.25, "bar", nil, nil, nil, 18 },
        { 14.5 * 1.25, SMALL, MUTED, item(index() + 1).title and fill(T("player.next"), { t = item(index() + 1).title })
                                     or T("player.last_song") },
    }
    local total = -10
    for _, l in ipairs(lines) do total = total + fs(l[1]) + 10 + (l[6] or 0) end
    local y = cy - total / 2
    for _, l in ipairs(lines) do
        y = y + (l[6] or 0)
        local h = fs(l[1])
        if l[2] == "bar" then
            progress(out, x, x + room, y + h / 2)
        elseif l[4] ~= "" then
            out[#out + 1] = text_ev(x, y + h / 2, 4, l[2], l[3], 1, fit(l[2], l[4], room), l[5])
        end
        y = y + h + 10
    end
end

local overlay = mp.create_osd_overlay("ass-events")
local shown_until = 0
local showing = false                 -- the last drawing had something on it
local margin = nil
local function visible() return paused() or mp.get_time() < shown_until end
local function render()
    if not drawn then return end
    local out = {}
    local now = mp.get_time()
    if not video then
        draw_song(out)
        local ax, ay, as = art_box()
        if paused() then draw_pause(out, ax + as / 2, ay + as / 2) end
        if visible() then prompts(out, RY - 41) end
    else
        if visible() then draw_video(out) end
        if paused() then draw_pause(out, RX / 2, RY / 2) end
    end
    if message ~= "" and now < message_until then draw_message(out) end
    overlay.res_x, overlay.res_y = RX, RY
    overlay.data = table.concat(out, "\n")
    overlay:update()
    showing = #out > 0
    -- a video's subtitles move up while the display shows
    local m = visible() and 200 or 22
    if video and m ~= margin then
        margin = m
        mp.set_property_number("sub-margin-y", m)
    end
end

-- every press shows the display for a moment
local function show()
    shown_until = mp.get_time() + 4
    render()
end
local function say(text)
    message, message_until = text, mp.get_time() + 1.8
    show()
end

-- Songs: the picture in its box (mpv draws it as the video), the console's background around it
local function place_picture()
    if video then return end
    local ax, ay, as = art_box()
    mp.set_property_number("video-margin-ratio-left", ax / RX)
    mp.set_property_number("video-margin-ratio-right", 1 - (ax + as) / RX)
    mp.set_property_number("video-margin-ratio-top", ay / RY)
    mp.set_property_number("video-margin-ratio-bottom", 1 - (ay + as) / RY)
    mp.set_property_number("panscan", 1)
    if drawn then mp.set_property("background-color", look.bg) end
end

local function resize()
    local d = mp.get_property_native("osd-dimensions") or {}
    if (d.w or 0) > 0 and (d.h or 0) > 0 then
        local s = math.min(d.w / 1280, d.h / 720)
        if math.abs(RX - d.w / s) > 0.01 or math.abs(RY - d.h / s) > 0.01 then widths = {} end
        RX, RY = d.w / s, d.h / s
    end
    place_picture()
    render()
end

---------------------------------------------------------------------------------------------------
-- The buttons ("Inside the player", [[controller-input-map]])

local function seek(by)
    mp.commandv("seek", tostring(by), "relative+exact")
    say(T(by > 0 and "player.forward" or "player.backward"))
end
-- LT: in the first 5 s the previous one; from 0:05 on (and on the first one), back to the start
local function previous()
    if position() >= 5 or index() <= 1 then
        mp.commandv("seek", "0", "absolute+exact")
        say(T("player.start"))
    else
        mp.commandv("playlist-prev")
    end
end
local function next_item()
    if index() >= count() then
        say(T(video and "player.last.video" or "player.last.song"))
        return
    end
    -- a video left half-way keeps its position (it resumes there, "Continue watching")
    if video and position() > 0 then mp.commandv("write-watch-later-config") end
    mp.commandv("playlist-next")
end
-- X: the next audio track, never "no sound"
local function audio()
    local list = tracks("audio")
    if not video or #list < 2 then show(); return end
    local _, i = current_track("audio")
    local n = i % #list + 1
    mp.set_property_number("aid", list[n].id)
    -- its language becomes the preferred one (a track without a language: none, each video's own)
    pref_audio = code_of(list[n])
    say(fill(T("player.audio"), { l = language(list[n], n), i = n, n = #list }))
    send_state()
end
-- Menu: subtitles off, then each track in turn
local function subtitles()
    local list = tracks("sub")
    if not video or #list == 0 then show(); return end
    local _, i = current_track("sub")
    if i >= #list then
        mp.set_property("sid", "no")
        pref_subs = "off"
        say(fill(T("player.subtitles"), { l = T("player.off") }))
    else
        mp.set_property_number("sid", list[i + 1].id)
        pref_subs = code_of(list[i + 1])   -- (no language: none, each video's own subtitles)
        say(fill(T("player.subtitles"), { l = language(list[i + 1], i + 1) }))
    end
    send_state()
end
-- A video opens with the preferred languages, before mpv chooses its tracks: the track in that
-- language, else the video's own default; subtitles "off" stay off (a .srt beside it too)
mp.add_hook("on_preloaded", 50, function()
    if not video then return end
    if pref_audio ~= "" then
        for _, t in ipairs(tracks("audio")) do
            if code_of(t) == pref_audio then mp.set_property_number("aid", t.id); break end
        end
    end
    if pref_subs == "off" then
        mp.set_property("sid", "no")
    elseif pref_subs ~= "" then
        for _, t in ipairs(tracks("sub")) do
            if code_of(t) == pref_subs then mp.set_property_number("sid", t.id); break end
        end
    end
end)
-- Y, Autoplay (videos), as Y in the Videos page: on, the next video of the folder starts when one
-- ends; off, the player stops at the end (keep-open) and goes back to the list
local function apply_autoplay() mp.set_property("keep-open", (video and not autoplay) and "always" or "no") end
local function toggle_autoplay()
    if not video then show(); return end
    autoplay = not autoplay
    apply_autoplay()
    say(fill(T("media.autoplay"), { state = T(autoplay and "media.on" or "media.off") }))
    send_state()
end
-- B: back to the list (mpv saves a video's position as it quits)
local function leave() mp.commandv("quit") end

-- Held buttons repeat with the map's timing (mpv's own repeat is one for every key): a
-- direction after 360 ms, then every 140 ms; LT / RT after 720 ms, then every 360 ms
local held = {}
local function stop_repeats()
    for name, t in pairs(held) do t:kill(); held[name] = nil end
end
local function bind(name, action, delay, every)
    mp.add_key_binding(nil, name, function(e)
        if e.event == "down" or e.event == "press" then
            if not on_screen then return end
            action()
            if delay and e.event == "down" then
                if held[name] then held[name]:kill() end
                held[name] = mp.add_timeout(delay, function()
                    held[name] = mp.add_periodic_timer(every, function()
                        if on_screen then action() else stop_repeats() end
                    end)
                end)
            end
        elseif e.event == "up" and held[name] then
            held[name]:kill()
            held[name] = nil
        end
    end, { complex = true })
end
bind("play-pause", function() mp.commandv("cycle", "pause") end)
bind("back-10", function() seek(-10) end, 0.36, 0.14)
bind("forward-10", function() seek(10) end, 0.36, 0.14)
bind("previous", previous, 0.72, 0.36)
bind("next", next_item, 0.72, 0.36)
bind("audio", audio)
bind("subtitles", subtitles)
bind("autoplay", toggle_autoplay)
bind("leave", leave)
bind("show", show)

---------------------------------------------------------------------------------------------------
-- What happens

-- owneetd: the console is on screen over the player ("no"), or the player again ("yes")
mp.register_script_message("owneet-focus", function(v)
    on_screen = v == "yes"
    if not on_screen then stop_repeats() else show() end
    send_state()
end)

mp.register_event("file-loaded", function()
    local t = mp.get_property_native("current-tracks/video")
    picture = not video and type(t) == "table"
    failed = ""
    place_picture()
    send_state()
    -- mpv used a saved position (videos only)
    if video and position() > 1 then say(fill(T("player.resumed"), { t = clock(position()) })) else show() end
end)
mp.register_event("end-file", function(e)
    if e.reason ~= "error" then return end
    -- the next file plays (mpv moves on); the last one: the player closes and the console says it
    for i, entry in ipairs(mp.get_property_native("playlist") or {}) do
        if entry.id == e.playlist_entry_id then
            failed = entry.filename or ""
            if i < count() then say(fill(T("player.failed"), { title = item(i).title or "" })) end
        end
    end
    send_state()
end)
-- Autoplay off: a video watched to the end goes back to the list, its position not kept (as if
-- never started)
mp.observe_property("eof-reached", "bool", function(_, v)
    if v and video and not autoplay then
        mp.set_property_bool("save-position-on-quit", false)
        mp.commandv("delete-watch-later-config")
        mp.commandv("quit")
    end
end)
mp.observe_property("pause", "bool", function()
    render()
    send_state()
end)
mp.register_event("playback-restart", function() if not on_screen then send_state() end end)
mp.observe_property("osd-dimensions", "native", resize)
-- the time on the display (until a drawing with nothing on it has cleared it); what plays for the
-- console's home screen while it is on screen
mp.add_periodic_timer(0.5, function()
    if not video or visible() or showing then render() end
end)
mp.add_periodic_timer(2, function() if not on_screen and not paused() then send_state() end end)

apply_autoplay()
