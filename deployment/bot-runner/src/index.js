require('dotenv').config();
const amqplib = require('amqplib');
const { runBotSession } = require('./runner');

const RABBITMQ_URL = process.env.RABBITMQ_URL || 'amqp://guest:guest@localhost:5672/';
const QUEUE_NAME = 'bot.google_meet.dispatch_queue';
const EXCHANGE_NAME = 'amq.topic';
const ROUTING_KEY = 'bot.google_meet.dispatch';

const activeSessions = new Map();

async function startDaemon() {
  console.log('[DAEMON] Starting Google Meet Bot Runner Daemon...');
  console.log(`[DAEMON] Connecting to RabbitMQ at ${RABBITMQ_URL}...`);

  let connection;
  while (!connection) {
    try {
      connection = await amqplib.connect(RABBITMQ_URL);
    } catch (err) {
      console.warn(`[DAEMON] RabbitMQ connection failed (${err.message}). Retrying in 5s...`);
      await new Promise((r) => setTimeout(r, 5000));
    }
  }

  const channel = await connection.createChannel();
  const exchanges = [process.env.RABBITMQ_EXCHANGE || 'app.events', 'amq.topic'];
  await channel.assertQueue(QUEUE_NAME, { durable: true });

  const STOP_ROUTING_KEY = 'bot.google_meet.stop';
  const stopQueue = await channel.assertQueue('', { exclusive: true });

  for (const ex of exchanges) {
    try {
      await channel.assertExchange(ex, 'topic', { durable: true });
      await channel.bindQueue(QUEUE_NAME, ex, ROUTING_KEY);
      await channel.bindQueue(stopQueue.queue, ex, STOP_ROUTING_KEY);
    } catch (err) {
      console.warn(`[DAEMON] Notice: failed to assert/bind to exchange "${ex}": ${err.message}`);
    }
  }

  // Prefetch 2 concurrent bot sessions per runner container
  const concurrency = parseInt(process.env.RUNNER_CONCURRENCY || '2', 10);
  await channel.prefetch(concurrency);

  channel.consume(stopQueue.queue, (msg) => {
    if (!msg) return;
    try {
      const payload = JSON.parse(msg.content.toString());
      const extId = payload.external_session_id;
      if (extId && activeSessions.has(extId)) {
        console.log(`[DAEMON] Received STOP signal for active session ${extId}. Initiating graceful exit...`);
        const token = activeSessions.get(extId);
        token.cancelled = true;
      }
    } catch (e) {
      console.error('[DAEMON ERROR] Failed to parse stop message:', e.message);
    } finally {
      channel.ack(msg);
    }
  });

  console.log(`[DAEMON READY] Listening on "${QUEUE_NAME}" (${ROUTING_KEY}) and stop broadcast (${STOP_ROUTING_KEY})...`);

  channel.consume(QUEUE_NAME, async (msg) => {
    if (!msg) return;

    let payload;
    try {
      payload = JSON.parse(msg.content.toString());
    } catch (e) {
      console.error('[DAEMON ERROR] Invalid JSON message payload:', msg.content.toString());
      channel.ack(msg);
      return;
    }

    const {
      session_id: sessionId,
      recording_id: recordingId,
      external_session_id: externalSessionId,
      meeting_url: meetingUrl,
    } = payload;

    if (!meetingUrl || !externalSessionId) {
      console.error('[DAEMON ERROR] Missing meeting_url or external_session_id:', payload);
      channel.ack(msg);
      return;
    }

    console.log(`[DAEMON] Received dispatch request for session ${externalSessionId} (${meetingUrl})`);

    const cancellationToken = { cancelled: false };
    activeSessions.set(externalSessionId, cancellationToken);

    try {
      await runBotSession(
        {
          sessionId,
          recordingId,
          externalSessionId,
          meetingUrl,
          apiEndpoint: process.env.API_ENDPOINT,
          webhookSecret: process.env.GOOGLE_MEET_BOT_WEBHOOK_SECRET,
        },
        cancellationToken
      );
    } catch (err) {
      console.error(`[DAEMON ERROR] Unhandled session error:`, err);
    } finally {
      activeSessions.delete(externalSessionId);
      channel.ack(msg);
    }
  });

  // Graceful shutdown
  const handleExit = async (signal) => {
    console.log(`[DAEMON] Received ${signal}. Terminating active sessions...`);
    for (const [id, token] of activeSessions.entries()) {
      console.log(`[DAEMON] Cancelling session ${id}...`);
      token.cancelled = true;
    }
    await channel.close().catch(() => {});
    await connection.close().catch(() => {});
    process.exit(0);
  };

  process.on('SIGTERM', () => handleExit('SIGTERM'));
  process.on('SIGINT', () => handleExit('SIGINT'));
}

startDaemon().catch((err) => {
  console.error('[FATAL DAEMON ERROR]:', err);
  process.exit(1);
});
