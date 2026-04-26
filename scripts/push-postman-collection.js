#!/usr/bin/env node

const fs = require('fs');
const path = require('path');

const apiKey = process.env.POSTMAN_API_KEY;
const collectionUID = process.env.POSTMAN_COLLECTION_UID;
const collectionPath = process.env.POSTMAN_COLLECTION_FILE
  ? path.resolve(process.cwd(), process.env.POSTMAN_COLLECTION_FILE)
  : path.resolve(__dirname, '..', 'docs', 'Wasla_Backend.postman_collection.json');

function fail(message) {
  console.error(message);
  process.exit(1);
}

function loadCollection(filePath) {
  if (!fs.existsSync(filePath)) {
    fail(`Collection file not found: ${filePath}`);
  }

  try {
    const raw = fs.readFileSync(filePath, 'utf8');
    const parsed = JSON.parse(raw);
    if (!parsed || typeof parsed !== 'object' || !parsed.info) {
      fail(`Invalid Postman collection JSON: ${filePath}`);
    }
    return parsed;
  } catch (err) {
    fail(`Failed to read collection file: ${err.message}`);
  }
}

async function pushCollection() {
  if (!apiKey) {
    fail('Missing POSTMAN_API_KEY.');
  }
  if (!collectionUID) {
    fail('Missing POSTMAN_COLLECTION_UID.');
  }

  const collection = loadCollection(collectionPath);
  const endpoint = `https://api.getpostman.com/collections/${encodeURIComponent(collectionUID)}`;

  const response = await fetch(endpoint, {
    method: 'PUT',
    headers: {
      'X-Api-Key': apiKey,
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({ collection }),
  });

  const rawBody = await response.text();
  if (!response.ok) {
    fail(`Postman API request failed (${response.status}): ${rawBody}`);
  }

  let payload = null;
  try {
    payload = JSON.parse(rawBody);
  } catch (_) {
    // Non-JSON success payload is unlikely, but do not fail after successful update.
  }

  const name = payload?.collection?.info?.name || collection.info.name || 'unknown';
  console.log(`Postman collection synced: ${name} (${collectionUID})`);
}

pushCollection().catch((err) => fail(`Unexpected error while pushing Postman collection: ${err.message}`));
