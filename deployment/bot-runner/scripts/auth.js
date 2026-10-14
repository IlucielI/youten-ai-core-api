const { spawn } = require('child_process');
const http = require('http');
const fs = require('fs');
const path = require('path');
const { chromium } = require('playwright-core');

const chromePaths = [
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
  '/usr/bin/google-chrome',
  '/usr/bin/chromium',
];

const executablePath = chromePaths.find(p => fs.existsSync(p));

if (!executablePath) {
  console.error('[AUTH ERROR] Google Chrome tidak ditemukan di path sistem.');
  console.error('Pastikan Google Chrome terinstall di Mac Anda (/Applications/Google Chrome.app).');
  process.exit(1);
}

const sessionDir = path.join(__dirname, '../session');
if (!fs.existsSync(sessionDir)) {
  fs.mkdirSync(sessionDir, { recursive: true });
}

const profileDir = path.join(sessionDir, 'chrome-auth-profile');
const storageStatePath = path.join(sessionDir, 'storage_state.json');
const cdpPort = 9222;

function waitForCDP(port, maxRetries = 30) {
  return new Promise((resolve, reject) => {
    let retries = 0;
    const interval = setInterval(() => {
      retries++;
      const req = http.get(`http://localhost:${port}/json/version`, res => {
        if (res.statusCode === 200) {
          clearInterval(interval);
          resolve(true);
        }
      });
      req.on('error', () => {
        if (retries >= maxRetries) {
          clearInterval(interval);
          reject(new Error('Gagal terhubung ke remote debugging Chrome setelah 15 detik.'));
        }
      });
    }, 500);
  });
}

(async () => {
  console.log('===========================================================');
  console.log('[AUTH] Membuka Google Chrome Asli (Native Mode Tanpa Otomasi)...');
  console.log('[AUTH] Mode ini melewati proteksi "This browser or app may not be secure".');
  console.log('[AUTH] Browser Chrome akan terbuka di layar Anda.');
  console.log('[AUTH] Silakan login ke akun Google Anda.');
  console.log('===========================================================');

  // Spawn official Google Chrome directly without any automation flags or playwright wrappers
  const chromeProcess = spawn(executablePath, [
    `--remote-debugging-port=${cdpPort}`,
    `--user-data-dir=${profileDir}`,
    '--no-first-run',
    '--no-default-browser-check',
    'https://accounts.google.com/',
  ]);

  chromeProcess.on('error', err => {
    console.error('[AUTH ERROR] Gagal menjalankan Chrome:', err.message);
    process.exit(1);
  });

  try {
    await waitForCDP(cdpPort);
    console.log('[AUTH] Terhubung ke Google Chrome via DevTools Protocol.');
  } catch (err) {
    console.error(err.message);
    chromeProcess.kill();
    process.exit(1);
  }

  // Connect Playwright over CDP to monitor login without triggering automation bot detection
  const browser = await chromium.connectOverCDP(`http://localhost:${cdpPort}`);
  const context = browser.contexts()[0] || (await browser.newContext());

  console.log('[AUTH] Silakan masukkan Email dan Password Anda di jendela Chrome...');
  console.log('[AUTH] Skrip akan otomatis mendeteksi ketika Anda telah selesai login.');

  let loggedIn = false;
  const timeoutMs = 5 * 60 * 1000; // 5 minutes
  const startTime = Date.now();

  while (!loggedIn && Date.now() - startTime < timeoutMs) {
    try {
      const cookies = await context.cookies();
      const hasSid = cookies.some(c => (c.name === 'SID' || c.name === 'SAPISID' || c.name === '__Secure-1PSID') && c.domain.includes('google.com'));

      const pages = context.pages();
      const hasAccountPage = pages.some(p => {
        const u = p.url();
        return u.includes('myaccount.google.com') || u.includes('google.com/?pli=1') || u.includes('meet.google.com');
      });

      if (hasSid || hasAccountPage) {
        loggedIn = true;
        break;
      }
    } catch (_) {}
    await new Promise(r => setTimeout(r, 2000));
  }

  if (!loggedIn) {
    console.error('[AUTH TIMEOUT] Waktu login melebihi 5 menit.');
    await browser.close().catch(() => {});
    chromeProcess.kill();
    process.exit(1);
  }

  console.log('[AUTH] Login terdeteksi! Mengambil token sesi...');
  await new Promise(r => setTimeout(r, 4000));

  await context.storageState({ path: storageStatePath });

  console.log('===========================================================');
  console.log(' [SUCCESS] Autentikasi Google Berhasil!');
  console.log(' Session tersimpan di: ' + storageStatePath);
  console.log(' Bot sekarang dapat bergabung ke Google Meet dengan akun ini.');
  console.log('===========================================================');

  await browser.close().catch(() => {});
  chromeProcess.kill();
  process.exit(0);
})();
