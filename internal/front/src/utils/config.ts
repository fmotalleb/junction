import YAML from 'yaml';

import TOML from "@iarna/toml";
import { NetworkConfig, EntryPoint } from '../types/config';

export const generateId = (): string => {
  return Math.random().toString(36).substr(2, 9);
};
export const createDefaultEntryPoint = (): EntryPoint => ({
  id: generateId(),
  routing: 'sni',
  listen: '',
  to: '',
  timeout: '1h',
  block_list: [],
  allow_list: [],
  proxy: [],

});


export type ConfigFormat = 'json' | 'yaml' | 'toml';

function getMap(self: EntryPoint) {
  const result: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(self)) {
    if (
      key !== 'id' &&
      value !== undefined &&
      value !== null &&
      value !== '' &&
      !(Array.isArray(value) && value.length === 0)
    ) {
      result[key] = value;
    }
  }
  return result;
}

function dumpCfg(cfg: NetworkConfig): any {
  const entrypoints = cfg.entrypoints.map(e => getMap(e));
  return { entrypoints };
}

export function exportConfig(config: NetworkConfig, format: ConfigFormat = 'json'): string {
  const cfg = dumpCfg(config)
  switch (format) {
    case 'json':
      return JSON.stringify(cfg, null, 2);
    case 'yaml':
      return YAML.stringify(cfg);
    case 'toml':
      return TOML.stringify(cfg);
    default:
      throw new Error(`Unsupported format: ${format}`);
  }
}

export const importConfig = (jsonString: string): NetworkConfig => {
  try {
    const parsed = JSON.parse(jsonString);
    if (!parsed.entrypoints || !Array.isArray(parsed.entrypoints)) {
      throw new Error('Invalid configuration format');
    }

    const entrypoints: EntryPoint[] = parsed.entrypoints.map((ep: any) => ({
      ...ep,
      id: generateId(),
      block_list: ep.block_list || [],
      allow_list: ep.allow_list || [],
      proxy: ep.proxy || []
    }));

    return { entrypoints };
  } catch (error) {
    throw new Error('Failed to parse configuration JSON');
  }
};

export const downloadFile = (content: string, filename: string, contentType: string = 'application/json') => {
  const blob = new Blob([content], { type: contentType });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  document.body.appendChild(link);
  link.click();
  document.body.removeChild(link);
  URL.revokeObjectURL(url);
};
