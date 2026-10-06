import { readFile, writeFile } from 'node:fs/promises';

const lockPath = process.argv[2];
if (!lockPath) {
  throw new Error('Usage: node patch-source-map-js-lock.mjs <package-lock.json>');
}

const lock = JSON.parse(await readFile(lockPath, 'utf8'));
const packageEntry = lock.packages?.['node_modules/source-map-js'];
if (!packageEntry) {
  console.log('source-map-js is not present in this lockfile.');
  process.exit(0);
}

const parseVersion = (version) => {
  const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(version ?? '');
  if (!match) throw new Error(`Unsupported source-map-js version: ${version}`);
  return match.slice(1).map(Number);
};
const isOlder = (left, right) => {
  const a = parseVersion(left);
  const b = parseVersion(right);
  for (let index = 0; index < a.length; index += 1) {
    if (a[index] !== b[index]) return a[index] < b[index];
  }
  return false;
};

const previousVersion = packageEntry.version;
if (!isOlder(previousVersion, '1.2.2')) {
  console.log(`source-map-js ${packageEntry.version} is already at or above the fixed version.`);
  process.exit(0);
}
if (previousVersion !== '1.2.1') {
  throw new Error(`Review required before patching source-map-js ${previousVersion}.`);
}

packageEntry.version = '1.2.2';
packageEntry.resolved = 'https://registry.npmjs.org/source-map-js/-/source-map-js-1.2.2.tgz';
packageEntry.integrity = 'sha512-KGj/8Y43x35aZVDtt+J4mK1hoLGHULMYfSkODJNQjNDC3oW1PqPoxMwo0pLUsWM/UEGzON/NxeHywEfNXNP3Vw==';
await writeFile(lockPath, `${JSON.stringify(lock, null, 2)}\n`);
console.log(`Updated source-map-js ${previousVersion} to 1.2.2 in ${lockPath}.`);
