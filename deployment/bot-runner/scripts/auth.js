const { chromium } = require('playwright-core');
const fs = require('fs');
const path = require('path');

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
const storageStatePath = path.join(sessionDir, 'storage_state.json');

(async () => {
  console.log('===========================================================');
  console.log('[AUTH] Membuka Google Chrome untuk autentikasi bot...');
  console.log('[AUTH] Silakan login dengan akun Google (akun bot / dummy / pribadi).');
  console.log('[AUTH] Jendela browser akan terbuka di layar Anda.');
  console.log('===========================================================');

  const browser = await chromium.launch({
    executablePath,
    headless: false,
    args: ['--no-first-run', '--no-default-browser-check'],
  });

  const context = await browser.newContext({
    viewport: { width: 1080, height: 720 },
  });

  const page = await context.newPage();
  await page.goto('https://accounts.google.com/');

  let loggedIn = false;
  const timeoutMs = 5 * 60 * 1000; // 5 minutes
  const startTime = Date.now();

  console.log('[AUTH] Menunggu Anda menyelesaikan login di browser...');

  while (!loggedIn && (Date.now() - startTime < timeoutMs)) {
    try {
      const cookies = await context.cookies();
      const hasSid = cookies.some(c => (c.name === 'SID' || c.name === 'SAPISID') && c.domain.includes('google.com'));
      const url = page.url();

      if (hasSid || url.includes('myaccount.google.com') || url.includes('google.com/?pli=1')) {
        loggedIn = true;
        break;
      }
    } catch (_) {}
    await page.waitForTimeout(2000);
  }

  if (!loggedIn) {
    console.error('[AUTH TIMEOUT] Waktu login melebihi batas 5 menit.');
    await browser.close();
    process.exit(1);
  }

  await page.waitForTimeout(3000);

  await context.storageState({ path: storageStatePath });
  console.log('===========================================================');
  console.log(' [SUCCESS] Autentikasi Google Berhasil!');
  console.log(' Session tersimpan di: ' + storageStatePath);
  console.log(' Bot sekarang dapat bergabung ke Google Meet dengan akun ini.');
  console.log('===========================================================');

  await browser.close();
  process.exit(0);
})();
