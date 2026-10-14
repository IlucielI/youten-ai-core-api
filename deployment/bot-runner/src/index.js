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
  await channel.assertExchange(EXCHANGE_NAME, 'topic', { durable: true });
  await channel.assertQueue(QUEUE_NAME, { durable: true });
  await channel.bindQueue(QUEUE_NAME, EXCHANGE_NAME, ROUTING_KEY);

  // Prefetch 2 concurrent bot sessions per runner container
  const concurrency = parseInt(process.env.RUNNER_CONCURRENCY || '2', 10);
  await channel.prefetch(concurrency);

  console.log(`[DAEMON READY] Listening on queue "${QUEUE_NAME}" for key "${ROUTING_KEY}" (concurrency: ${concurrency})...`);

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
