// SPDX-License-Identifier: GPL-3.0-or-later
// A game's art with rounded corners: its own image from local files when there is one (cropped to
// fill), otherwise soft colour fields from a hue derived from the title, so every game keeps the
// same colours (no third-party images). `shade` darkens the bottom (for a title) and `scrim` the
// left side (for text over the art). With `fit` the image is shown whole, over the generated art,
// at `fitX` / `fitY` (0…1: where the free room goes), e.g. a Steam game's wide header in a
// portrait cover, or a game's portrait cover in the home screen's wide card (step 4.4).
import QtQuick 2.15

Canvas {
    id: root
    property string seed: ""
    property url image: ""
    property real radius: Theme.radiusM
    property bool shade: false
    property bool scrim: false
    property bool fit: false
    property real fitX: 0.5
    property real fitY: 0.5
    property real fitMargin: Theme.px(10)
    readonly property real hue: {        // FNV-1a hash of the title, spread over the colour wheel
        let h = 2166136261;
        for (let i = 0; i < seed.length; i++)
            h = Math.imul(h ^ seed.charCodeAt(i), 16777619) >>> 0;
        return h % 360;
    }
    property bool imageReady: false

    onHueChanged: requestPaint()
    onWidthChanged: requestPaint()
    onHeightChanged: requestPaint()
    // An image another art has loaded already gives no imageLoaded signal: ready at once
    function useImage() {
        const src = image.toString();
        imageReady = src !== "" && isImageLoaded(src);
        if (src !== "" && !imageReady)
            loadImage(src);
        requestPaint();
    }
    onImageChanged: useImage()
    onImageLoaded: { imageReady = image.toString() !== "" && isImageLoaded(image.toString()); requestPaint(); }
    Component.onCompleted: useImage()

    function hsl(h, s, l, a) { return Qt.hsla((((h % 360) + 360) % 360) / 360, s, l, a === undefined ? 1 : a); }
    function rounded(ctx, w, h, r) {
        ctx.beginPath();
        ctx.moveTo(r, 0); ctx.arcTo(w, 0, w, h, r); ctx.arcTo(w, h, 0, h, r);
        ctx.arcTo(0, h, 0, 0, r); ctx.arcTo(0, 0, w, 0, r); ctx.closePath();
    }
    onFitChanged: requestPaint()
    onPaint: {
        const ctx = getContext("2d");
        const w = width, h = height;
        ctx.reset();
        ctx.save();
        rounded(ctx, w, h, Math.min(radius, w / 2, h / 2));
        ctx.clip();
        // (the Canvas functions take the image's address as text, not as a url)
        const img = imageReady ? ctx.createImageData(image.toString()) : null;
        if (img && img.width > 0 && !fit) {
            const s = Math.max(w / img.width, h / img.height);   // crop to fill
            const iw = img.width * s, ih = img.height * s;
            ctx.drawImage(image.toString(), (w - iw) / 2, (h - ih) / 2, iw, ih);
        } else {
            let g = ctx.createLinearGradient(0, 0, w, h);
            g.addColorStop(0, hsl(hue, 0.45, 0.26));
            g.addColorStop(1, hsl(hue + 25, 0.50, 0.12));
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, w, h);
            g = ctx.createRadialGradient(w * 0.8, h * 0.1, 0, w * 0.8, h * 0.1, Math.max(w, h) * 0.7);
            g.addColorStop(0, hsl(hue, 0.85, 0.62, 0.95));
            g.addColorStop(1, hsl(hue, 0.85, 0.62, 0));
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, w, h);
            g = ctx.createRadialGradient(0, h, 0, 0, h, Math.max(w, h) * 0.65);
            g.addColorStop(0, hsl(hue + 40, 0.70, 0.35, 1));
            g.addColorStop(1, hsl(hue + 40, 0.70, 0.35, 0));
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, w, h);
        }
        if (img && img.width > 0 && fit) {      // the whole image, over the generated art
            const m = fitMargin;
            const s = Math.min((w - 2 * m) / img.width, (h - 2 * m) / img.height);
            const iw = img.width * s, ih = img.height * s;
            const x = m + (w - 2 * m - iw) * fitX, y = m + (h - 2 * m - ih) * fitY;
            ctx.save();
            ctx.translate(x, y);
            rounded(ctx, iw, ih, Math.min(radius / 2, iw / 2, ih / 2));
            ctx.clip();
            ctx.drawImage(image.toString(), 0, 0, iw, ih);
            ctx.restore();
        }
        if (shade) {
            const g = ctx.createLinearGradient(0, h * 0.45, 0, h);
            g.addColorStop(0, "rgba(0,0,0,0)");
            g.addColorStop(1, "rgba(0,0,0,0.6)");
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, w, h);
        }
        if (scrim) {
            const g = ctx.createLinearGradient(0, 0, w * 0.7, 0);
            g.addColorStop(0, "rgba(0,0,0,0.55)");
            g.addColorStop(1, "rgba(0,0,0,0)");
            ctx.fillStyle = g;
            ctx.fillRect(0, 0, w, h);
        }
        ctx.restore();
    }
}
