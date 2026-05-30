import { ScreenshotMode } from './types';

export interface Preset {
  name: string;
  host: string;
  pathRegex?: string;
  primary: string;
  fallbacks?: string[];
  tags?: string[];
  mode?: ScreenshotMode;
  topPx?: number;
}

// Ready-made profiles for commonly monitored CTI / underground forums.
// Selectors are best-effort starting points keyed to each forum's engine
// (XenForo, MyBB, IPB); refine any of them with the visual picker.
export const PRESETS: Preset[] = [
  {
    name: 'XSS.is', host: 'xss.is', pathRegex: '/threads?/.*',
    primary: 'h1.p-title-value', fallbacks: ['h1', "meta[property='og:title']"],
    tags: ['cti', 'forum', 'xss'], mode: 'top_px', topPx: 2600,
  },
  {
    name: 'Exploit.in', host: 'exploit.in',
    primary: 'h1', fallbacks: ['.threadtitle', "meta[property='og:title']", 'title'],
    tags: ['cti', 'forum', 'exploit'], mode: 'top_px', topPx: 2400,
  },
  {
    name: 'BreachForums', host: 'breachforums.st', pathRegex: '/Thread-.*',
    primary: "meta[property='og:title']", fallbacks: ['span.thread-title', 'h1', 'title'],
    tags: ['cti', 'forum', 'breach'],
  },
  {
    name: 'Nulled', host: 'nulled.to', pathRegex: '/topic/.*',
    primary: '.ipsType_pageTitle', fallbacks: ['h1 span', 'h1', "meta[property='og:title']"],
    tags: ['cti', 'forum', 'nulled'],
  },
  {
    name: 'Cracked.io', host: 'cracked.io', pathRegex: '/Thread-.*',
    primary: "meta[property='og:title']", fallbacks: ['span.thread-title', 'h1', 'title'],
    tags: ['cti', 'forum', 'cracked'],
  },
  {
    name: 'Leakbase', host: 'leakbase.io', pathRegex: '/threads?/.*',
    primary: 'h1.p-title-value', fallbacks: ['h1', "meta[property='og:title']"],
    tags: ['cti', 'forum', 'leak'],
  },
  {
    name: 'Hackforums', host: 'hackforums.net', pathRegex: '/Thread-.*',
    primary: "meta[property='og:title']", fallbacks: ['span.thread-title', 'title'],
    tags: ['cti', 'forum'],
  },
];
