const { S3Client, PutObjectCommand } = require('@aws-sdk/client-s3');
const fs = require('fs');

/**
 * Initializes S3/MinIO client from environment variables.
 */
function createS3Client() {
  let endpoint = process.env.S3_ENDPOINT || process.env.MINIO_ENDPOINT || 'http://localhost:9000';
  if (!endpoint.startsWith('http://') && !endpoint.startsWith('https://')) {
    endpoint = `http://${endpoint}`;
  }
  const region = process.env.S3_REGION || 'us-east-1';
  const accessKeyId = process.env.S3_ACCESS_KEY || process.env.MINIO_ROOT_USER || 'minioadmin';
  const secretAccessKey = process.env.S3_SECRET_KEY || process.env.MINIO_ROOT_PASSWORD || 'minioadmin';
  const forcePathStyle = true; // MinIO compatibility

  return new S3Client({
    endpoint,
    region,
    credentials: {
      accessKeyId,
      secretAccessKey,
    },
    forcePathStyle,
  });
}

/**
 * Uploads a local audio file to S3/MinIO.
 * Returns the object storage key (e.g., recordings/{session_id}/audio.wav).
 */
async function uploadAudioFile(filePath, objectKey) {
  const s3 = createS3Client();
  const bucket = process.env.S3_BUCKET_NAME || 'code-base-golang';

  const fileStream = fs.createReadStream(filePath);
  const fileStats = fs.statSync(filePath);

  console.log(`[STORAGE] Uploading ${filePath} (${fileStats.size} bytes) to bucket "${bucket}" with key "${objectKey}"...`);

  const command = new PutObjectCommand({
    Bucket: bucket,
    Key: objectKey,
    Body: fileStream,
    ContentType: 'audio/wav',
    ContentLength: fileStats.size,
  });

  await s3.send(command);
  console.log(`[STORAGE SUCCESS] Audio uploaded successfully: ${objectKey}`);
  return objectKey;
}

module.exports = { uploadAudioFile };
