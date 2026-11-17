const { chromium } = require('playwright-core');
const { spawn } = require('child_process');
const fs = require('fs');
const path = require('path');
const { sendWebhook } = require('./webhook');
const { uploadAudioFile } = require('./storage');

/**
 * Executes an automated Google Meet bot session.
 */
async function runBotSession(params, cancellationToken = { cancelled: false }) {
  const {
    sessionId,
    recordingId,
    externalSessionId,
    meetingUrl,
    apiEndpoint = process.env.API_ENDPOINT || 'http://localhost:8080',
    webhookSecret = process.env.GOOGLE_MEET_BOT_WEBHOOK_SECRET || '',
    botName = process.env.GOOGLE_MEET_BOT_NAME || 'AI Meeting Assistant',
    admitTimeoutSec = parseInt(process.env.GOOGLE_MEET_ADMIT_TIMEOUT_SECONDS || '600', 10),
    maxMeetingDurationSec = parseInt(process.env.GOOGLE_MEET_MAX_DURATION_SECONDS || '7200', 10),
    emptyRoomGraceSec = parseInt(process.env.GOOGLE_MEET_EMPTY_ROOM_TIMEOUT_SECONDS || '180', 10),
  } = params;

  console.log(`[RUNNER] Launching Google Meet bot for session ${externalSessionId} on URL: ${meetingUrl}`);

  const tempAudioPath = path.join('/tmp', `${externalSessionId}.wav`);
  let browser = null;
  let ffmpegProcess = null;
  let startTime = null;

  try {
    // 1. Notify API: Waiting admit
    await sendWebhook(apiEndpoint, webhookSecret, {
      external_session_id: externalSessionId,
      event: 'waiting_admit',
    });

    // 2. Launch Chromium browser with headless flags and media stream spoofing
    const executablePath = process.env.CHROMIUM_PATH || '/usr/bin/chromium' || '/usr/bin/google-chrome';
    browser = await chromium.launch({
      executablePath: fs.existsSync(executablePath) ? executablePath : undefined,
      headless: process.env.HEADLESS !== 'false',
      args: [
        '--use-fake-ui-for-media-stream',
        '--use-fake-device-for-media-stream',
        '--disable-notifications',
        '--autoplay-policy=no-user-gesture-required',
        '--no-sandbox',
        '--disable-setuid-sandbox',
        '--disable-dev-shm-usage',
        '--disable-blink-features=AutomationControlled',
        '--window-size=1280,720',
      ],
    });

    const storageStatePath = process.env.GOOGLE_STORAGE_STATE_PATH || path.join(__dirname, '../session/storage_state.json');
    const contextOptions = {
      permissions: ['microphone', 'camera'],
      userAgent: 'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36',
      locale: 'id-ID',
      timezoneId: 'Asia/Jakarta',
    };

    if (fs.existsSync(storageStatePath)) {
      console.log(`[RUNNER] Loading authenticated Google session from ${storageStatePath}`);
      contextOptions.storageState = storageStatePath;
    } else {
      console.log('[RUNNER NOTICE] No Google session file found; attempting entry as guest.');
    }

    const context = await browser.newContext(contextOptions);

    await context.addInitScript(() => {
      Object.defineProperty(navigator, 'webdriver', { get: () => undefined });
      window.chrome = { runtime: {} };
      Object.defineProperty(navigator, 'plugins', { get: () => [1, 2, 3, 4, 5] });
      Object.defineProperty(navigator, 'languages', { get: () => ['id-ID', 'id', 'en-US', 'en'] });
    });

    const page = await context.newPage();
    page.setDefaultTimeout(30000);

    console.log(`[RUNNER] Navigating to ${meetingUrl}...`);
    await page.goto(meetingUrl, { waitUntil: 'domcontentloaded', timeout: 45000 });
    await page.waitForTimeout(4000);

    const currentUrl = page.url();
    const pageTitle = await page.title().catch(() => '');
    console.log(`[RUNNER] Loaded page: ${currentUrl} | Title: "${pageTitle}"`);

    // Check if redirected to Google Accounts login
    if (currentUrl.includes('accounts.google.com')) {
      throw new Error(`Google Meet redirected to sign-in page (${currentUrl}). This meeting requires a signed-in Google account.`);
    }

    // Dismiss common Google dialogs (cookie consent, notifications, "Got it")
    const dismissSelectors = [
      'button:has-text("Accept all")',
      'button:has-text("Setuju semua")',
      'button:has-text("I agree")',
      'button:has-text("Saya setuju")',
      'button:has-text("Got it")',
      'button:has-text("Mengerti")',
      'button:has-text("Dismiss")',
      'button:has-text("Tutup")',
      'div[role="button"]:has-text("Got it")',
      'div[role="button"]:has-text("Mengerti")',
    ];
    for (const sel of dismissSelectors) {
      try {
        const el = page.locator(sel).first();
        if (await el.isVisible({ timeout: 1000 })) {
          console.log(`[RUNNER] Dismissing dialog with selector: ${sel}`);
          await el.click().catch(() => {});
          await page.waitForTimeout(1000);
        }
      } catch (_) {}
    }

    // 3. Mute microphone and turn off camera before joining (without toggling back)
    try {
      // Toggle off microphone if currently enabled
      const micSelectors = [
        '[aria-label*="turn off microphone" i]',
        '[aria-label*="matikan mikrofon" i]',
        'button[data-is-muted="false"][aria-label*="mic" i]',
        'div[role="button"][data-is-muted="false"][aria-label*="mic" i]',
      ];
      for (const sel of micSelectors) {
        const btn = page.locator(sel).first();
        if (await btn.isVisible({ timeout: 800 }).catch(() => false)) {
          console.log(`[RUNNER] Muting microphone before joining via selector: ${sel}`);
          await btn.click().catch(() => {});
          break;
        }
      }

      // Toggle off camera if currently enabled
      const camSelectors = [
        '[aria-label*="turn off camera" i]',
        '[aria-label*="matikan kamera" i]',
        'button[data-is-muted="false"][aria-label*="camera" i]',
        'div[role="button"][data-is-muted="false"][aria-label*="camera" i]',
      ];
      for (const sel of camSelectors) {
        const btn = page.locator(sel).first();
        if (await btn.isVisible({ timeout: 800 }).catch(() => false)) {
          console.log(`[RUNNER] Turning off camera before joining via selector: ${sel}`);
          await btn.click().catch(() => {});
          break;
        }
      }
    } catch (e) {
      console.warn('[RUNNER] Pre-join media mute warning:', e.message);
    }
    await page.waitForTimeout(1000);

    // 4. Input bot guest name if prompt is present
    const nameSelectors = [
      'input[placeholder*="name" i]',
      'input[placeholder*="nama" i]',
      'input[aria-label*="name" i]',
      'input[aria-label*="nama" i]',
      'input[type="text"]',
    ];
    let nameFilled = false;
    for (const sel of nameSelectors) {
      try {
        const input = page.locator(sel).first();
        if (await input.isVisible({ timeout: 2000 })) {
          await input.click();
          await input.fill(botName);
          console.log(`[RUNNER] Filled bot name (${botName}) using selector: ${sel}`);
          nameFilled = true;
          await page.waitForTimeout(1500);
          break;
        }
      } catch (_) {}
    }
    if (!nameFilled) {
      console.log('[RUNNER] Name input not required or already filled.');
    }

    // 5. Locate and click "Ask to join", "Join now", "Join here too", or "Switch here"
    const joinSelectors = [
      'button:has-text("Ask to join")',
      'button:has-text("Join now")',
      'button:has-text("Join here too")',
      'button:has-text("Switch here")',
      'button:has-text("Minta bergabung")',
      'button:has-text("Gabung sekarang")',
      'button:has-text("Gabung di sini juga")',
      'button:has-text("Beralih ke sini")',
      'div[role="button"]:has-text("Ask to join")',
      'div[role="button"]:has-text("Join now")',
      'div[role="button"]:has-text("Join here too")',
      'div[role="button"]:has-text("Switch here")',
      'div[role="button"]:has-text("Minta bergabung")',
      'div[role="button"]:has-text("Gabung sekarang")',
      'div[role="button"]:has-text("Gabung di sini juga")',
      'div[role="button"]:has-text("Beralih ke sini")',
      'span:has-text("Ask to join")',
      'span:has-text("Join now")',
      'span:has-text("Join here too")',
      'span:has-text("Switch here")',
      'span:has-text("Minta bergabung")',
      'span:has-text("Gabung sekarang")',
      'span:has-text("Gabung di sini juga")',
      'span:has-text("Beralih ke sini")',
      'button.UywwFc-LgbsSe',
      '[aria-label*="Ask to join" i]',
      '[aria-label*="Join now" i]',
      '[aria-label*="Join here too" i]',
      '[aria-label*="Switch here" i]',
      '[aria-label*="Minta bergabung" i]',
      '[aria-label*="Gabung sekarang" i]',
      '[aria-label*="Gabung di sini juga" i]',
      '[aria-label*="Beralih ke sini" i]',
    ];

    // Dismiss any preview overlays (e.g., camera permission or upgrade prompts)
    const previewDismissSelectors = [
      'button:has-text("Dismiss")',
      'button:has-text("Tutup")',
      'div[role="button"]:has-text("Dismiss")',
      'div[role="button"]:has-text("Tutup")',
      '[aria-label*="close" i]',
      '[aria-label*="tutup" i]',
    ];
    for (const dSel of previewDismissSelectors) {
      try {
        const el = page.locator(dSel).first();
        if (await el.isVisible({ timeout: 500 }).catch(() => false)) {
          console.log(`[RUNNER] Dismissing preview dialog via: ${dSel}`);
          await el.click({ force: true }).catch(() => {});
          await page.waitForTimeout(500);
        }
      } catch (_) {}
    }

    let clicked = false;
    const findJoinDeadline = Date.now() + 15000;
    while (Date.now() < findJoinDeadline && !clicked) {
      for (const sel of joinSelectors) {
        try {
          const btn = page.locator(sel).first();
          if (await btn.isVisible({ timeout: 1000 })) {
            console.log(`[RUNNER] Found join button with selector: ${sel}. Clicking...`);
            try {
              await btn.click({ timeout: 3000, force: true });
            } catch (clickErr) {
              console.warn('[RUNNER] Normal click failed, falling back to DOM click:', clickErr.message);
              await btn.evaluate(el => el.click());
            }
            clicked = true;
            break;
          }
        } catch (e) {
          console.warn(`[RUNNER] Error checking selector ${sel}:`, e.message);
        }
      }

      if (!clicked) {
        // Direct DOM evaluation fallback to find and click any join/switch button
        const clickedText = await page.evaluate(() => {
          const candidates = Array.from(document.querySelectorAll('button, div[role="button"], a[role="button"]'));
          for (const btn of candidates) {
            const txt = (btn.innerText || '').toLowerCase();
            if (
              txt.includes('join now') ||
              txt.includes('join here') ||
              txt.includes('gabung sekarang') ||
              txt.includes('gabung di sini') ||
              txt.includes('switch here') ||
              txt.includes('beralih ke sini') ||
              txt.includes('ask to join') ||
              txt.includes('minta bergabung')
            ) {
              btn.click();
              return btn.innerText.trim();
            }
          }
          return null;
        }).catch(() => null);

        if (clickedText) {
          console.log(`[RUNNER] Found and clicked join button via in-DOM fallback: "${clickedText}"`);
          clicked = true;
          break;
        }

        await page.waitForTimeout(1500);
      }
    }

    if (!clicked) {
      // Diagnostic dump before failing
      const visibleTexts = await page.evaluate(() => {
        return {
          buttons: Array.from(document.querySelectorAll('button, div[role="button"]')).map(b => b.innerText.trim()).filter(Boolean).slice(0, 15),
          inputs: Array.from(document.querySelectorAll('input')).map(i => i.placeholder || i.getAttribute('aria-label') || i.name).slice(0, 10),
          bodySnippet: document.body ? document.body.innerText.replace(/\s+/g, ' ').trim().slice(0, 300) : ''
        };
      }).catch(() => ({}));
      console.error('[RUNNER DIAGNOSTIC] Page UI elements:', JSON.stringify(visibleTexts));
      throw new Error(`Could not find Join or Ask to Join button on Google Meet preview page. UI State: ${JSON.stringify(visibleTexts)}`);
    }

    console.log('[RUNNER] Successfully clicked Join / Ask to join button. Waiting for host admission...');

    // 6. Wait for Admission or Denial
    const admissionStart = Date.now();
    let isAdmitted = false;
    let lastLogged = 0;

    while (Date.now() - admissionStart < admitTimeoutSec * 1000) {
      if (cancellationToken.cancelled) {
        throw new Error('Bot session cancelled by user');
      }

      await page.screenshot({ path: '/tmp/latest_meet.png' }).catch(() => {});

      if (Date.now() - lastLogged > 8000) {
        lastLogged = Date.now();
        const snippet = await page.evaluate(() => document.body ? document.body.innerText.replace(/\s+/g, ' ').trim().slice(0, 400) : '').catch(() => '');
        console.log(`[RUNNER LOBBY STATE] "${snippet}"`);
      }

      // Check and dismiss any overlay dialogs in lobby (e.g., "Meet has been blocked", "Login with Google", "Got it")
      const lobbyOverlayDismiss = [
        'button:has-text("Got it")',
        'button:has-text("Mengerti")',
        'button:has-text("Oke")',
        'button:has-text("OK")',
        'button:has-text("Dismiss")',
        'button:has-text("Tutup")',
        'div[role="button"][aria-label*="close" i]',
        'button[aria-label*="close" i]',
      ];
      for (const oSel of lobbyOverlayDismiss) {
        const oBtn = page.locator(oSel).first();
        if (await oBtn.isVisible({ timeout: 300 }).catch(() => false)) {
          console.log(`[RUNNER] Dismissing dialog via: ${oSel}`);
          await oBtn.click({ force: true }).catch(() => {});
          await page.keyboard.press('Escape').catch(() => {});
          await page.waitForTimeout(400);
        }
      }

      // If Join / Ask to join button is still visible on preview, re-click it via DOM click
      const pendingJoin = page.locator('button:has-text("Join now"), button:has-text("Join here too"), button:has-text("Gabung sekarang"), button:has-text("Gabung di sini juga"), button:has-text("Ask to join"), button:has-text("Minta bergabung")').first();
      if (await pendingJoin.isVisible({ timeout: 500 }).catch(() => false)) {
        console.log('[RUNNER] Join button still visible on page, triggering DOM click...');
        await page.keyboard.press('Escape').catch(() => {});
        await pendingJoin.evaluate(el => el.click()).catch(() => {});
        await page.waitForTimeout(1000);
      }

      // Check if admitted: Meeting call controls appear or in-call elements exist
      const inCallDetected = await page.evaluate(() => {
        const text = (document.body ? document.body.innerText : '').toLowerCase();
        const hasCallControls = !!document.querySelector(
          '[aria-label*="Leave" i], [aria-label*="Keluar" i], [aria-label*="reaction" i], [aria-label*="Raise hand" i], [aria-label*="Share screen" i], button[data-call-ended]'
        );
        const hasInCallText = (text.includes('turn on microphone') || text.includes('turn off microphone')) &&
                              (text.includes('turn on camera') || text.includes('turn off camera'));
        return hasCallControls || hasInCallText;
      }).catch(() => false);

      if (inCallDetected) {
        console.log('[RUNNER] In-call controls detected! Admitted to meeting.');
        isAdmitted = true;
        // Dismiss any intro popups (e.g., "Others may see your video differently")
        await page.keyboard.press('Escape').catch(() => {});
        await page.evaluate(() => {
          const gotIt = Array.from(document.querySelectorAll('button')).find(b => {
            const t = (b.innerText || '').toLowerCase();
            return t.includes('got it') || t.includes('mengerti');
          });
          if (gotIt) gotIt.click();
        }).catch(() => {});
        break;
      }

      // Check if redirected away from meeting room or kicked to landing screen
      const currentLoopUrl = page.url();
      const meetingCode = meetingUrl.split('/').pop().split('?')[0];
      if (!currentLoopUrl.includes(meetingCode) || currentLoopUrl.includes('workspace.google.com')) {
        throw new Error(`Bot was redirected away from meeting room (${currentLoopUrl})`);
      }

      // Check if denied by host or blocked by room security
      const deniedSelectors = [
        'text="Anda tidak dapat bergabung"',
        'text="You can\'t join"',
        'text="Someone denied your request"',
        'text="Permintaan Anda untuk bergabung ditolak"',
        'text="Penyelenggara menolak"',
      ];
      for (const dSel of deniedSelectors) {
        if (await page.locator(dSel).first().isVisible().catch(() => false)) {
          throw new Error(`Bot admission rejected by Google Meet security policy: ${dSel}`);
        }
      }

      await page.waitForTimeout(2000);
    }

    if (!isAdmitted) {
      throw new Error(`Timed out waiting for admission after ${admitTimeoutSec} seconds`);
    }

    console.log('[RUNNER] Successfully admitted into Google Meet!');
    startTime = Date.now();

    // Verify and ensure microphone and camera are muted inside the call
    try {
      const inCallMic = page.locator('[aria-label*="turn off microphone" i], [aria-label*="matikan mikrofon" i]').first();
      if (await inCallMic.isVisible({ timeout: 1500 }).catch(() => false)) {
        console.log('[RUNNER] In-call microphone active. Muting...');
        await inCallMic.click().catch(() => {});
      }
      const inCallCam = page.locator('[aria-label*="turn off camera" i], [aria-label*="matikan kamera" i]').first();
      if (await inCallCam.isVisible({ timeout: 1500 }).catch(() => false)) {
        console.log('[RUNNER] In-call camera active. Turning off...');
        await inCallCam.click().catch(() => {});
      }
    } catch (_) {}

    // 7. Notify API: Joined
    await sendWebhook(apiEndpoint, webhookSecret, {
      external_session_id: externalSessionId,
      event: 'joined',
    });

    // 8. Start Audio Recording via PulseAudio virtual sink
    console.log(`[RUNNER] Starting audio capture to ${tempAudioPath}...`);
    ffmpegProcess = startAudioRecording(tempAudioPath);

    // Notify API: Recording
    await sendWebhook(apiEndpoint, webhookSecret, {
      external_session_id: externalSessionId,
      event: 'recording',
    });

    // 9. Meeting Active Monitoring Loop
    let emptyRoomTimer = 0;

    while (!cancellationToken.cancelled) {
      const elapsedSec = (Date.now() - startTime) / 1000;
      if (elapsedSec > maxMeetingDurationSec) {
        console.log(`[RUNNER] Maximum meeting duration reached (${maxMeetingDurationSec}s). Leaving.`);
        break;
      }

      // Detect if host ended call or kicked bot
      const leftNotice = page.locator('text="You left the meeting", text="You\'ve been removed from the meeting", text="Panggilan telah berakhir"').first();
      if (await leftNotice.isVisible().catch(() => false)) {
        console.log('[RUNNER] Call ended or bot removed from room.');
        break;
      }

      // Detect empty room (only bot remaining)
      const participantCount = await getParticipantCount(page);
      if (participantCount !== null && participantCount <= 1) {
        emptyRoomTimer += 5;
        if (emptyRoomTimer >= emptyRoomGraceSec) {
          console.log(`[RUNNER] Room empty for ${emptyRoomGraceSec}s. Concluding session.`);
          break;
        }
      } else {
        emptyRoomTimer = 0;
      }

      await page.waitForTimeout(5000);
    }

    // 10. Conclude Recording and Upload
    // Explicitly click "Leave call" button to exit meeting immediately
    try {
      console.log('[RUNNER] Clicking "Leave call" button to disconnect bot from room...');
      const leaveBtn = page.locator('[aria-label*="Leave call" i], [aria-label*="Keluar dari panggilan" i], button[aria-label*="call" i]').first();
      if (await leaveBtn.isVisible({ timeout: 1500 }).catch(() => false)) {
        await leaveBtn.click().catch(() => {});
        await page.waitForTimeout(400);
      }
      await page.evaluate(() => {
        const btns = Array.from(document.querySelectorAll('button, div[role="button"]'));
        const leave = btns.find(b => {
          const l = (b.getAttribute('aria-label') || '').toLowerCase();
          return l.includes('leave') || l.includes('keluar') || l.includes('end call');
        });
        if (leave) leave.click();
      }).catch(() => {});
    } catch (_) {}

    // Close browser right away so bot disappears from meeting instantly
    if (browser) {
      console.log('[RUNNER] Closing browser immediately...');
      await browser.close().catch(() => {});
      browser = null;
    }

    const durationSeconds = Math.max(1, Math.round((Date.now() - startTime) / 1000));
    await stopAudioRecording(ffmpegProcess, tempAudioPath);

    const s3ObjectKey = `recordings/${sessionId}/audio.wav`;
    await uploadAudioFile(tempAudioPath, s3ObjectKey);

    // 11. Notify API: Completed
    await sendWebhook(apiEndpoint, webhookSecret, {
      external_session_id: externalSessionId,
      event: 'completed',
      audio_url: s3ObjectKey,
      duration_seconds: durationSeconds,
    });

    console.log(`[RUNNER SUCCESS] Session ${externalSessionId} completed in ${durationSeconds} seconds.`);
  } catch (err) {
    console.error(`[RUNNER ERROR] Session ${externalSessionId} failed:`, err.message);

    if (ffmpegProcess) {
      try { ffmpegProcess.kill('SIGKILL'); } catch (_) {}
    }

    await sendWebhook(apiEndpoint, webhookSecret, {
      external_session_id: externalSessionId,
      event: 'failed',
      error_message: err.message,
    });
  } finally {
    if (browser) {
      await browser.close().catch(() => {});
    }
    if (fs.existsSync(tempAudioPath)) {
      try { fs.unlinkSync(tempAudioPath); } catch (_) {}
    }
  }
}

/**
 * Spawns ffmpeg to record audio from PulseAudio monitor or generates a standard audio wav file.
 */
function startAudioRecording(outputPath) {
  try {
    const { execSync } = require('child_process');
    execSync('pulseaudio --check || pulseaudio -D --exit-idle-time=-1', { stdio: 'ignore' });
  } catch (_) {}

  // Try PulseAudio first, fallback to generating standard WAV header with silence/sine if no audio sink
  const isPulseAvailable = fs.existsSync('/var/run/pulse') || process.env.PULSE_SERVER;

  if (isPulseAvailable) {
    return spawn('ffmpeg', [
      '-y',
      '-f', 'pulse',
      '-i', 'default',
      '-ac', '1',
      '-ar', '16000',
      '-c:a', 'pcm_s16le',
      outputPath,
    ], { stdio: 'ignore' });
  }

  // Local dev / test fallback: Record virtual audio device or dummy loopback (with -re for real-time pacing)
  return spawn('ffmpeg', [
    '-y',
    '-re',
    '-f', 'lavfi',
    '-i', 'sine=frequency=1000:sample_rate=16000:beep_factor=4',
    '-c:a', 'pcm_s16le',
    outputPath,
  ], { stdio: 'ignore' });
}

/**
 * Stops ffmpeg recording and waits for file to finalize.
 */
function stopAudioRecording(proc, outputPath) {
  return new Promise((resolve) => {
    if (!proc) {
      // Ensure file exists as fallback
      if (!fs.existsSync(outputPath)) {
        fs.writeFileSync(outputPath, Buffer.alloc(44)); // minimal WAV header
      }
      return resolve();
    }

    proc.on('exit', () => resolve());
    proc.kill('SIGINT');

    // Force kill if it doesn't close within 5 seconds
    setTimeout(() => {
      try { proc.kill('SIGKILL'); } catch (_) {}
      resolve();
    }, 5000);
  });
}

/**
 * Extracts participant count from Google Meet UI if visible.
 */
async function getParticipantCount(page) {
  try {
    const participantButton = page.locator('[aria-label*="Show everyone" i], [aria-label*="Tampilkan semua orang" i]').first();
    if (await participantButton.isVisible().catch(() => false)) {
      const text = await participantButton.textContent().catch(() => '');
      const match = text.match(/\d+/);
      if (match) return parseInt(match[0], 10);
    }
  } catch (_) {}
  return null;
}

module.exports = { runBotSession };
