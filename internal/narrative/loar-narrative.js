#!/usr/bin/env node
'use strict';

// loar-narrative.js — deterministic NLG for Loar context packages.
// Reads a JSON context package from stdin, writes narrative prose to stdout.
// No npm dependencies required.

const chunks = [];
process.stdin.on('data', d => chunks.push(d));
process.stdin.on('end', () => {
  try {
    const pkg = JSON.parse(Buffer.concat(chunks).toString('utf8'));
    process.stdout.write(render(pkg) + '\n');
  } catch (e) {
    process.stderr.write('loar-narrative: ' + e.message + '\n');
    process.exit(1);
  }
});

// pick — deterministic selection from an array based on a string seed.
// Same query always picks the same opener, but different queries vary.
function pick(arr, seed) {
  const h = seed.split('').reduce((a, c) => (Math.imul(a, 31) + c.charCodeAt(0)) | 0, 0);
  return arr[Math.abs(h) % arr.length];
}

function plural(n, singular, plur) {
  return n === 1 ? `1 ${singular}` : `${n} ${plur}`;
}

function render(pkg) {
  const { query, entities, observations, contradictions, date_range } = pkg;
  const lines = [];
  const entityNames = (entities || []).map(e => e.canonical_name || e.name).filter(Boolean);
  const heading = query;

  lines.push(heading);
  lines.push('─'.repeat(40));
  lines.push('');

  const obs = observations || [];
  if (obs.length === 0) {
    lines.push('No evidence found for this query.');
    return lines.join('\n');
  }

  // Lead sentence — varies by query, anchored to entity name when resolved.
  const n = obs.length;
  const openers = entityNames.length > 0 ? [
    `${entityNames[0]} appears in ${plural(n, 'observation', 'observations')} in the knowledge store.`,
    `The knowledge store holds ${plural(n, 'observation', 'observations')} relating to ${entityNames[0]}.`,
    `${plural(n, 'observation', 'observations')} found for ${entityNames[0]}.`,
  ] : [
    `The knowledge store holds ${plural(n, 'observation', 'observations')} relevant to this query.`,
    `${plural(n, 'observation', 'observations')} found for this query.`,
  ];
  lines.push(pick(openers, query));
  lines.push('');

  // Declare all state variables before use.
  const seenContent = new Set();
  const grouped = {};
  const undated = [];

  // Pick lead: best non-UUID dated observation that names an entity.
  // Penalise self-repeating records (boundary/policy records that echo
  // the same sentence twice) and excessively long ones.
  let lead = null;
  let leadScore = -1;
  for (const o of obs) {
    const d = o.occurred_at ? o.occurred_at.slice(0, 10) : null;
    if (!d) continue;
    const content = cleanContent(o.content);
    if (content.length < 40) continue;
    if (uuidDensity(o.content) > 0.15) continue;
    // Skip records that repeat their first 50 chars later in the same content.
    const snippet = content.slice(0, 50).toLowerCase().trim();
    if (snippet.length > 20 && content.toLowerCase().lastIndexOf(snippet) > 55) continue;
    // Score: peak at 200 chars, taper above 350 (overly long = noisier).
    const len = content.length;
    const lengthScore = len <= 200 ? len / 200 : Math.max(0.1, 1 - (len - 200) / 600);
    let score = lengthScore;
    if (entityNames.some(nm => content.toLowerCase().includes(nm.toLowerCase()))) score += 0.5;
    if (score > leadScore) { leadScore = score; lead = o; }
  }

  // Group remaining observations by date (YYYY-MM-DD), undated go last.
  for (const o of obs) {
    if (lead && o === lead) continue; // skip lead from date groups
    const content = cleanContent(o.content);
    if (content.length < 40) continue;
    if (uuidDensity(o.content) > 0.15) continue;
    // Skip records that are still too noisy after cleaning (excessive punctuation noise).
    if ((content.match(/[,;]/g) || []).length > content.split(' ').length * 0.4) continue;
    const d = o.occurred_at ? o.occurred_at.slice(0, 10) : null;
    if (d) {
      if (!grouped[d]) grouped[d] = [];
      grouped[d].push(o);
    } else {
      undated.push(o);
    }
  }

  // Lead observation — best dated, entity-mentioning, non-UUID record.
  // Show only the first 2 sentences so trailing metadata fields don't appear.
  if (lead) {
    const content = cleanContent(lead.content);
    const leadText = firstTwoSentences(content, 280);
    lines.push(wordWrap(leadText, 72));
    lines.push('');
    seenContent.add(content);
  }

  for (const d of Object.keys(grouped).sort()) {
    const bullets = [];
    for (const o of grouped[d]) {
      const content = cleanContent(o.content);
      if (content.length < 40) continue;
      const line = firstSentence(content, 200);
      if (isDuplicate(line, seenContent)) continue;
      seenContent.add(content);
      bullets.push(line);
      if (bullets.length >= 5) break;
    }
    if (bullets.length === 0) continue;
    lines.push(d);
    for (const b of bullets) lines.push('  • ' + b);
    lines.push('');
  }

  // Undated observations (no date header).
  const undatedBullets = [];
  for (const o of undated) {
    const content = cleanContent(o.content);
    if (content.length < 40) continue;
    const line = firstSentence(content, 160);
    if (isDuplicate(line, seenContent)) continue;
    seenContent.add(content);
    undatedBullets.push(line);
    if (undatedBullets.length >= 3) break;
  }
  if (undatedBullets.length > 0) {
    for (const b of undatedBullets) lines.push('  • ' + b);
    lines.push('');
  }

  // Contradiction — one only, surfaced as a tension.
  if (contradictions && contradictions.length > 0) {
    const parts = contradictions[0].summary.split('  ↔  ');
    if (parts.length === 2) {
      const a = firstSentence(cleanContent(parts[0]), 180).replace(/…$/, '').trim();
      const b = firstSentence(cleanContent(parts[1]), 180).replace(/…$/, '').trim();
      lines.push(`⚡ ${a}`);
      lines.push(`   → ${b}`);
      lines.push('');
    }
  }

  // Footer.
  const dr = date_range || {};
  const dateStr = (dr.earliest && dr.latest && dr.earliest !== dr.latest)
    ? ` · ${dr.earliest} → ${dr.latest}`
    : (dr.earliest ? ` · ${dr.earliest}` : '');
  lines.push(`Based on ${plural(n, 'observation', 'observations')}${dateStr}`);

  return lines.join('\n');
}

// cleanContent strips field-name prefixes injected by buildContent,
// removes UUID tokens, ISO timestamps, collapses comma-noise, and normalises whitespace.
// Handles both snake_case (alternatives_considered:) and title-case multi-word prefixes.
function cleanContent(s) {
  // Strip snake_case and camelCase/PascalCase field names (no spaces).
  s = s.replace(/^\w[\w_]*:\s*/gm, '');
  // Strip title-case multi-word labels up to 3 words.
  s = s.replace(/^(?:[A-Za-z][a-z]+ ){1,3}[A-Za-z][a-z]+:\s*/gm, '');
  // Strip UUID-shaped tokens (with optional trailing punctuation).
  s = s.replace(/\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}[,;.]?\b/gi, '');
  // Strip ISO-8601 timestamp tokens.
  s = s.replace(/\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d+)?Z?\b/g, '');
  // Collapse 2+ consecutive commas/semicolons (artifacts of empty-array concatenation).
  s = s.replace(/(?:[,;]\s*){2,}/g, '');
  return s.replace(/\n/g, ' ').replace(/\s+/g, ' ').trim();
}

// firstTwoSentences returns up to max chars, ending at the second sentence
// boundary when possible. Used for lead paragraphs to prevent trailing
// metadata tokens from appearing.
function firstTwoSentences(s, max) {
  const chars = [...s];
  if (chars.length <= max) return s;
  let sentences = 0;
  for (let i = 20; i < Math.min(max, chars.length); i++) {
    const c = chars[i];
    if (c === '!' || c === '?') {
      sentences++;
      if (sentences >= 2) return chars.slice(0, i + 1).join('');
    }
    if (c === '.' && (i + 1 >= chars.length || chars[i + 1] === ' ')) {
      sentences++;
      if (sentences >= 2) return chars.slice(0, i + 1).join('');
    }
  }
  // Fewer than 2 sentence boundaries found within max — use firstSentence.
  return firstSentence(s, max);
}

// firstSentence returns up to max chars, ending at a sentence boundary when
// possible. A '.' is only treated as a boundary when followed by a space or
// end-of-string, preventing breaks on file extensions and decimals.
// Avoids truncating inside [...] bracket tokens.
function firstSentence(s, max) {
  if (s.length <= max) return s;
  const chars = [...s]; // unicode-safe
  for (let i = 20; i < Math.min(max, chars.length); i++) {
    const c = chars[i];
    if (c === '!' || c === '?') return chars.slice(0, i + 1).join('');
    if (c === '.' && (i + 1 >= chars.length || chars[i + 1] === ' ')) {
      return chars.slice(0, i + 1).join('');
    }
  }
  // Hard truncation: back up to avoid cutting inside a [...] token.
  let cut = max;
  const tail = chars.slice(0, max + 20).join('');
  const openBracket = tail.lastIndexOf('[', max);
  const closeBracket = tail.indexOf(']', max);
  if (openBracket > max - 12 && (closeBracket === -1 || closeBracket > max)) {
    // We'd cut inside a bracket — back up to before the bracket.
    cut = openBracket > 20 ? openBracket : max;
  }
  return chars.slice(0, cut).join('') + '…';
}

// isDuplicate returns true if line is already covered by a seen entry
// (exact or prefix match, case-insensitive).
function isDuplicate(line, seen) {
  const lower = line.toLowerCase();
  for (const s of seen) {
    const sl = s.toLowerCase();
    if (sl.startsWith(lower) || lower.startsWith(sl)) return true;
  }
  return false;
}

// uuidDensity returns the fraction of tokens that look like UUIDs.
function uuidDensity(s) {
  const tokens = s.split(/\s+/);
  if (tokens.length === 0) return 0;
  const uuidRe = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12},?$/i;
  const count = tokens.filter(t => uuidRe.test(t)).length;
  return count / tokens.length;
}

// wordWrap breaks s into lines of at most width chars at word boundaries.
function wordWrap(s, width) {
  const words = s.split(' ');
  const lines = [];
  let current = '';
  for (const w of words) {
    if (current.length + (current ? 1 : 0) + w.length > width) {
      if (current) lines.push(current);
      current = w;
    } else {
      current = current ? current + ' ' + w : w;
    }
  }
  if (current) lines.push(current);
  return lines.join('\n');
}
