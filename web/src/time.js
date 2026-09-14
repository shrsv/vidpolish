// timeAgo turns a Unix timestamp (seconds) into a short relative string
// ("3 minutes ago", "2 days ago") for compact display. fullTimestamp
// renders the same timestamp in full, for a hover title so the exact
// moment is always one hover away.
const UNITS = [
  ['year', 31536000],
  ['month', 2592000],
  ['week', 604800],
  ['day', 86400],
  ['hour', 3600],
  ['minute', 60],
];

export function timeAgo(unixSeconds) {
  if (!unixSeconds) return '';
  const seconds = Math.floor(Date.now() / 1000) - unixSeconds;
  if (seconds < 5) return 'just now';
  if (seconds < 60) return `${seconds}s ago`;
  for (const [name, unitSeconds] of UNITS) {
    const value = Math.floor(seconds / unitSeconds);
    if (value >= 1) return `${value} ${name}${value > 1 ? 's' : ''} ago`;
  }
  return 'just now';
}

export function fullTimestamp(unixSeconds) {
  if (!unixSeconds) return '';
  return new Date(unixSeconds * 1000).toLocaleString(undefined, {
    dateStyle: 'medium',
    timeStyle: 'short',
  });
}
