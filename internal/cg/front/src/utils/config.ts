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
  proxy: []
});

export const exportConfig = (config: NetworkConfig): string => {
  const exportData = {
    entrypoints: config.entrypoints
      .map(({ id, ...rest }) => rest),
  };
  return JSON.stringify(exportData, null, 2);
};

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