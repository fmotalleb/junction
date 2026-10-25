import YAML from 'yaml';

import TOML from "@iarna/toml";
import { NetworkConfig, EntryPoint } from '../types/config';

export const generateId = (): string => {
   return Math.random().toString(36).substring(2, 11); 
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

/**
 * Returns a plain object representation of an EntryPoint, excluding the `id` property and any properties that are undefined, null, empty strings, or empty arrays.
 *
 * @param self - The EntryPoint object to process
 * @returns An object containing only the non-empty, non-id properties of the EntryPoint
 */
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

/**
 * Converts a `NetworkConfig` object into a plain object with entry points mapped to simplified representations.
 *
 * @returns An object containing an array of entry points, each processed to exclude empty or undefined fields except for the `id`.
 */
function dumpCfg(cfg: NetworkConfig): Record<string, unknown> {
  const entrypoints = cfg.entrypoints.map(e => getMap(e));
  return { ...cfg, entrypoints };
}

/**
 * Serializes a network configuration into a string in the specified format.
 *
 * @param config - The network configuration to serialize
 * @param format - The output format: 'json', 'yaml', or 'toml' (defaults to 'json')
 * @returns The serialized configuration as a string in the chosen format
 * @throws If an unsupported format is specified
 */
export function exportConfig(config: NetworkConfig, format: ConfigFormat = 'json'): string {
  const cfg = dumpCfg(config)
  switch (format) {
    case 'json':
      return JSON.stringify(cfg, null, 2);
    case 'yaml':
      return YAML.stringify(cfg);
    case 'toml':
      return TOML.stringify(cfg as Parameters<typeof TOML.stringify>[0]);
    default:
      throw new Error(`Unsupported format: ${format}`);
  }
}

export const importConfig = (jsonString: string): NetworkConfig => {
  try {
    if (new TextEncoder().encode(jsonString).length > 4 * 1024 * 1024) {
      throw new Error('Configuration exceeds the 4 MiB import limit');
    }
    const parsed: unknown = JSON.parse(jsonString);
    if (!isRecord(parsed) || !Array.isArray(parsed.entrypoints)) {
      throw new Error('Invalid configuration format');
    }

    const entrypoints: EntryPoint[] = parsed.entrypoints.map((value: unknown) => {
      if (!isRecord(value)) {
        throw new Error('Each entry point must be an object');
      }
      for (const field of ['block_list', 'allow_list', 'allow_from', 'block_from', 'destination_allow', 'destination_block', 'proxy', 'features']) {
        if (value[field] !== undefined && (!Array.isArray(value[field]) || !value[field].every((item) => typeof item === 'string'))) {
          throw new Error(`${field} must be an array of strings`);
        }
      }
      const routing = typeof value.routing === 'string' ? value.routing : '';
      if (!['sni', 'http-header', 'http-to-https', 'tcp-raw', 'udp-raw'].includes(routing)) {
        throw new Error('Each entry point must have a supported routing type');
      }
      return {
        ...value,
        id: generateId(),
        routing: routing as EntryPoint['routing'],
        listen: typeof value.listen === 'string' ? value.listen : '',
        to: typeof value.to === 'string' ? value.to : '',
        block_list: (value.block_list as string[] | undefined) ?? [],
        allow_list: (value.allow_list as string[] | undefined) ?? [],
        allow_from: (value.allow_from as string[] | undefined) ?? [],
        block_from: (value.block_from as string[] | undefined) ?? [],
        proxy: (value.proxy as string[] | undefined) ?? [],
      };
    });

    return { ...parsed, entrypoints } as NetworkConfig;
  } catch (error: unknown) {
    throw new Error(error instanceof Error ? error.message : 'Failed to parse configuration JSON');
  }
};

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

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
