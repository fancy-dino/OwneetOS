// SPDX-License-Identifier: GPL-3.0-or-later
//
// Relays messages between the page (content/input.js) and owneet-bridge, the native-messaging host
// that talks to owneetd ([[extension-design]]). Brave starts the bridge on the first connection;
// a connected native port keeps this worker alive.

'use strict';

const HOST = 'org.owneet.bridge';
const pages = new Set();
let bridge = null, look = null, keyboardPage = null;

function toPages(msg) {
  if (msg.type === 'look') look = msg;
  if (msg.type === 'text' || msg.type === 'key' || msg.type === 'keyboard_closed') {
    if (keyboardPage) keyboardPage.postMessage(msg);
    if (msg.type === 'keyboard_closed') keyboardPage = null;
    return;
  }
  pages.forEach(p => p.postMessage(msg));
}

function connectBridge() {
  if (bridge) return;
  try {
    bridge = chrome.runtime.connectNative(HOST);
  } catch (_) {
    bridge = null;
    return;
  }
  bridge.onMessage.addListener(toPages);
  bridge.onDisconnect.addListener(() => {
    void chrome.runtime.lastError;            // read, so that Brave does not log it as unchecked
    bridge = null;
    // The keyboard can no longer close through the bridge: give the controller back to the page
    if (keyboardPage) { keyboardPage.postMessage({ type: 'keyboard_closed' }); keyboardPage = null; }
    if (pages.size) setTimeout(connectBridge, 5000);
  });
  bridge.postMessage({ type: 'hello' });
}

// A supported web app drawn as on a screen msg.width px wide (Spotify on a TV, [[extension-design]]
// "Spotify"): Brave's zoom for the page's tab, from what the page measures at the zoom it has now,
// between 100 % and 300 % in steps of 5 %
async function zoomTab(page, msg) {
  const tab = page.sender && page.sender.tab;
  if (!tab || !(msg.width > 0) || !(msg.innerWidth > 0)) return;
  try {
    const now = await chrome.tabs.getZoom(tab.id);
    const want = Math.min(3, Math.max(1, Math.round(now * msg.innerWidth / msg.width * 20) / 20));
    if (Math.abs(want - now) >= 0.05) await chrome.tabs.setZoom(tab.id, want);
  } catch (_) { /* the tab went away */ }
}

chrome.runtime.onConnect.addListener(page => {
  if (page.name !== 'page') return;
  pages.add(page);
  connectBridge();
  if (look) page.postMessage(look);
  page.onMessage.addListener(msg => {
    if (msg.type === 'zoom') { zoomTab(page, msg); return; }
    if (msg.type === 'keyboard' && msg.open) {
      if (!bridge) { page.postMessage({ type: 'keyboard_closed' }); return; }
      keyboardPage = page;
    }
    if (bridge) bridge.postMessage(msg);
  });
  page.onDisconnect.addListener(() => {
    pages.delete(page);
    if (keyboardPage === page) {
      keyboardPage = null;
      if (bridge) bridge.postMessage({ type: 'keyboard', open: false });
    }
  });
});
