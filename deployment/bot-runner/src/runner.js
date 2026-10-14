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

    const context = await browser.newContext({
      permissions: ['microphone', 'camera'],
      userAgent: 'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/122.0.0.0 Safari/537.36',
    });

    const page = await context.newPage();
    page.setDefaultTimeout(30000);

    console.log(`[RUNNER] Navigating to ${meetingUrl}...`);
    await page.goto(meetingUrl, { waitUntil: 'networkidle' });

    // 3. Mute microphone and turn off camera before joining
    await page.waitForTimeout(2000);
    try {
      await page.keyboard.press('Control+d'); // Toggle microphone off
      await page.keyboard.press('Control+e'); // Toggle camera off
    } catch (e) {
      console.warn('[RUNNER] Keyboard shortcuts mute warning:', e.message);
    }

    // 4. Input bot guest name if prompt is present
    try {
      const nameInput = page.locator('input[type="text"], input[aria-label*="name" i], #c11').first();
      if (await nameInput.isVisible({ timeout: 4000 })) {
        await nameInput.fill(botName);
        console.log(`[RUNNER] Filled bot name: ${botName}`);
      }
    } catch (e) {
      console.log('[RUNNER] Name input step skipped or already signed in.');
    }

    // 5. Click "Ask to join" or "Join now"
    const joinButton = page.locator(
      'button:has-text("Ask to join"), button:has-text("Join now"), button:has-text("Minta bergabung"), button:has-text("Gabung sekarang"), [aria-label*="Join" i]'
    ).first();

    await joinButton.waitFor({ state: 'visible', timeout: 10000 });
    await joinButton.click();
    console.log('[RUNNER] Clicked Join / Ask to join button.');

    // 6. Wait for Admission or Denial
    const admissionStart = Date.now();
    let isAdmitted = false;

    while (Date.now() - admissionStart < admitTimeoutSec * 1000) {
      if (cancellationToken.cancelled) {
        throw new Error('Bot session cancelled by user');
      }

      // Check if admitted: Meeting call controls appear
      const inCallControl = page.locator('[aria-label*="Leave call" i], [aria-label*="Keluar dari panggilan" i], button[aria-label*="call" i]').first();
      if (await inCallControl.isVisible().catch(() => false)) {
        isAdmitted = true;
        break;
      }

      // Check if denied by host
      const deniedMessage = page.locator('text="You can\'t join this call", text="Someone denied your request to join", text="Permintaan Anda untuk bergabung ditolak"').first();
      if (await deniedMessage.isVisible().catch(() => false)) {
        throw new Error('Bot admission denied by meeting host');
      }

      await page.waitForTimeout(2000);
    }

    if (!isAdmitted) {
      throw new Error(`Timed out waiting for admission after ${admitTimeoutSec} seconds`);
    }

    console.log('[RUNNER] Successfully admitted into Google Meet!');
    startTime = Date.now();

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
      fs.unlinkSync(tempAudioPath);
    }
  }
}

/**
 * Spawns ffmpeg to record audio from PulseAudio monitor or generates a standard audio wav file.
 */
function startAudioRecording(outputPath) {
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

  // Local dev / test fallback: Record virtual audio device or dummy loopback
  return spawn('ffmpeg', [
    '-y',
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
