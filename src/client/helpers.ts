export type ExtensionLike = {
  id: string;
  packageJSON?: {
    activationEvents?: unknown;
    contributes?: {
      languages?: Array<{ id?: string }>;
    };
    displayName?: string;
  };
};

export function formatDuration(durationMs: number): string {
  if (!Number.isFinite(durationMs) || durationMs < 0) {
    return '0 ms';
  }
  if (durationMs < 1000) {
    return `${Math.round(durationMs).toLocaleString()} ms`;
  }
  return `${(durationMs / 1000).toFixed(durationMs < 10_000 ? 2 : 1)}s`;
}

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) {
    return '0 B';
  }
  const units = ['B', 'KB', 'MB', 'GB'];
  let value = bytes;
  let unitIndex = 0;
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024;
    unitIndex++;
  }
  const decimals = unitIndex === 0 ? 0 : 1;
  return `${value.toFixed(decimals)} ${units[unitIndex]}`;
}

export function formatRate(value: number): string {
  if (!Number.isFinite(value) || value < 0) {
    return '0';
  }
  return Math.round(value).toLocaleString();
}

export function mergeExcludePatterns(...groups: readonly string[][]): string[] {
  const merged = new Set<string>();
  for (const group of groups) {
    for (const pattern of group) {
      const trimmed = pattern.trim();
      if (trimmed) {
        merged.add(trimmed);
      }
    }
  }
  return [...merged];
}

export function isPotentialPhpConflict(ext: ExtensionLike): boolean {
  const id = ext.id.toLowerCase();
  const activationEvents = Array.isArray(ext.packageJSON?.activationEvents)
    ? ext.packageJSON.activationEvents
    : [];
  const contributesLanguages = Array.isArray(ext.packageJSON?.contributes?.languages)
    ? ext.packageJSON.contributes.languages
    : [];

  if (id === 'vscode.php-language-features') {
    return true;
  }

  if (!contributesLanguages.some((language: { id?: string }) => language?.id === 'php')) {
    return false;
  }

  return activationEvents.some((event: string) => event === 'onLanguage:php');
}
