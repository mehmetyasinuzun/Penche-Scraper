import { AppConfig, DomainProfile, defaultConfig } from './types';

const STORAGE_KEY = 'penche_config';

/** Load config from browser.storage.sync, merging with defaults. */
export async function loadConfig(): Promise<AppConfig> {
  const result = await browser.storage.sync.get(STORAGE_KEY);
  const stored = result[STORAGE_KEY] as AppConfig | undefined;
  if (!stored) return defaultConfig();

  // Forward-compatible: merge missing top-level keys from default.
  const def = defaultConfig();
  return {
    version: stored.version ?? def.version,
    global: { ...def.global, ...stored.global },
    domains: stored.domains ?? def.domains,
  };
}

/** Persist the full config. */
export async function saveConfig(cfg: AppConfig): Promise<void> {
  await browser.storage.sync.set({ [STORAGE_KEY]: cfg });
}

/** Save or update a single domain profile. */
export async function saveDomainProfile(domain: string, profile: DomainProfile): Promise<void> {
  const cfg = await loadConfig();
  cfg.domains[domain] = profile;
  await saveConfig(cfg);
}

/** Delete a domain profile. */
export async function deleteDomainProfile(domain: string): Promise<void> {
  const cfg = await loadConfig();
  delete cfg.domains[domain];
  await saveConfig(cfg);
}

/** Find the best matching profile for a given host + path. */
export function resolveProfile(
  cfg: AppConfig,
  host: string,
  path: string
): { profileId: string; profile: DomainProfile } | null {
  // Exact host match first
  for (const [key, profile] of Object.entries(cfg.domains)) {
    if (!profile.enabled) continue;
    if (profile.match.host !== host) continue;
    if (profile.match.pathRegex) {
      try {
        const re = new RegExp(profile.match.pathRegex);
        if (!re.test(path)) continue;
      } catch {
        // Invalid regex in profile — skip regex check.
      }
    }
    return { profileId: key, profile };
  }

  // Wildcard / subdomain match (*.onion or *.domain.com)
  for (const [key, profile] of Object.entries(cfg.domains)) {
    if (!profile.enabled) continue;
    const matchHost = profile.match.host;
    if (matchHost.startsWith('*.')) {
      const suffix = matchHost.slice(2);
      if (host.endsWith(suffix)) {
        return { profileId: key, profile };
      }
    }
  }

  return null;
}

/** Export config as a JSON string for backup. */
export async function exportConfig(): Promise<string> {
  const cfg = await loadConfig();
  return JSON.stringify(cfg, null, 2);
}

/** Import config from a JSON string, validating structure. */
export async function importConfig(json: string): Promise<void> {
  const parsed: AppConfig = JSON.parse(json);
  if (typeof parsed.version !== 'number') throw new Error('Invalid config: missing version');
  if (!parsed.global) throw new Error('Invalid config: missing global');
  if (!parsed.domains) throw new Error('Invalid config: missing domains');
  await saveConfig(parsed);
}
