const keywords = new Set(
  'as async await break case catch class const continue default delete do else export extends false finally for from function if import in interface let new null of package private protected public return select static struct switch this throw true try type undefined var void while yield'.split(' '),
);

function escapeHTML(value: string): string {
  return value.replace(/[&<>"']/g, (char) => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]!);
}

function languageFor(path: string): 'markup' | 'python' | 'plain' | 'code' {
  const extension = path.split('.').at(-1)?.toLowerCase();
  if (['html', 'htm', 'svelte', 'xml', 'svg'].includes(extension ?? '')) return 'markup';
  if (['py', 'sh', 'bash', 'yaml', 'yml', 'toml'].includes(extension ?? '')) return 'python';
  if (['md', 'txt', 'log', 'csv'].includes(extension ?? '')) return 'plain';
  return 'code';
}

/** Small, dependency-free syntax coloring for the read-only file inspector. */
export function highlightSource(source: string, path: string): string {
  const language = languageFor(path);
  const tokenPattern = language === 'markup'
    ? /<!--[\s\S]*?-->|<\/?[A-Za-z][^>]*>|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'/g
    : language === 'plain'
      ? /$^/g
      : language === 'python'
        ? /\/\*[\s\S]*?\*\/|#[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b(?:\d+(?:\.\d+)?|[A-Za-z_$][\w$]*)\b/g
        : /\/\*[\s\S]*?\*\/|\/\/[^\n]*|"(?:\\.|[^"\\])*"|'(?:\\.|[^'\\])*'|`(?:\\.|[^`\\])*`|\b(?:\d+(?:\.\d+)?|[A-Za-z_$][\w$]*)\b/g;

  let output = '';
  let offset = 0;
  for (const match of source.matchAll(tokenPattern)) {
    const token = match[0];
    const start = match.index ?? offset;
    output += escapeHTML(source.slice(offset, start));
    let type = '';
    if (token.startsWith('<!--') || token.startsWith('/*') || token.startsWith('//') || (language === 'python' && token.startsWith('#'))) {
      type = 'syntax-comment';
    } else if (language === 'markup' && token.startsWith('<')) {
      type = 'syntax-tag';
    } else if (/^["'`]/.test(token)) {
      type = 'syntax-string';
    } else if (/^\d/.test(token)) {
      type = 'syntax-number';
    } else if (keywords.has(token)) {
      type = 'syntax-keyword';
    }
    output += type ? `<span class="${type}">${escapeHTML(token)}</span>` : escapeHTML(token);
    offset = start + token.length;
  }
  return output + escapeHTML(source.slice(offset));
}
