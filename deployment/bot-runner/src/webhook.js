/**
 * Webhook client for dispatching asynchronous lifecycle status updates to ai-core-api.
 */

async function sendWebhook(endpoint, secret, payload) {
  const url = endpoint.endsWith('/') 
    ? `${endpoint}v1/webhooks/bot/google-meet` 
    : `${endpoint}/v1/webhooks/bot/google-meet`;

  console.log(`[WEBHOOK] Sending event ${payload.event} for session ${payload.external_session_id} to ${url}`);

  const maxRetries = 3;
  let attempt = 0;

  while (attempt < maxRetries) {
    attempt++;
    try {
      const res = await fetch(url, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'X-Bot-Webhook-Secret': secret || '',
        },
        body: JSON.stringify(payload),
      });

      if (!res.ok) {
        const errorText = await res.text().catch(() => '');
        throw new Error(`HTTP ${res.status}: ${errorText}`);
      }

      const data = await res.json().catch(() => ({}));
      console.log(`[WEBHOOK SUCCESS] Event ${payload.event} processed:`, data.message || 'OK');
      return true;
    } catch (err) {
      console.error(`[WEBHOOK ATTEMPT ${attempt} FAILED] ${err.message}`);
      if (attempt < maxRetries) {
        await new Promise((r) => setTimeout(r, 2000 * attempt));
      } else {
        console.error(`[WEBHOOK ERROR] Exhausted retries for event ${payload.event}`);
        return false;
      }
    }
  }
}

module.exports = { sendWebhook };
