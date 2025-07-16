import React, { useState } from 'react';
import { NetworkConfig } from '../types/config';
import { exportConfig, ConfigFormat } from '../utils/config';

interface JsonPreviewProps {
  config: NetworkConfig;
}

export const JsonPreview: React.FC<JsonPreviewProps> = ({ config }) => {
  const [format, setFormat] = useState<ConfigFormat>('toml');
  const [copied, setCopied] = useState(false);
  const formattedConfig = exportConfig(config, format);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(formattedConfig);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      console.error('Copy failed:', err);
    }
  };

  return (
    <div className="bg-gray-900/80 backdrop-blur-xl rounded-lg border border-gray-700/50 p-4 overflow-auto max-h-96">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium text-gray-300 uppercase">{format} Preview</h3>
        <div className="flex items-center gap-4">
          {/* Format switch buttons */}
          <div className="inline-flex border border-gray-700 rounded overflow-hidden">
            {(['toml', 'yaml', 'json'] as ConfigFormat[]).map((f) => (
              <button
                key={f}
                onClick={() => setFormat(f)}
                className={`text-xs px-3 py-1 transition-colors ${
                  format === f
                    ? 'bg-emerald-600 text-white'
                    : 'bg-gray-800 text-gray-300 hover:bg-gray-700'
                }`}
              >
                {f.toUpperCase()}
              </button>
            ))}
          </div>

          {/* Copy button */}
          <button
            onClick={handleCopy}
            className="text-xs text-gray-300 border border-gray-600 rounded px-2 py-1 hover:bg-gray-700"
          >
            {copied ? 'Copied' : 'Copy'}
          </button>

          {/* Status */}
          <div className="flex items-center gap-1">
            <div className="w-2 h-2 bg-gradient-to-r from-emerald-400 to-green-500 rounded-full shadow-lg shadow-emerald-500/50"></div>
            <span className="text-xs text-gray-400">Valid</span>
          </div>
        </div>
      </div>

      <pre className="text-sm text-gray-300 font-mono leading-relaxed whitespace-pre-wrap">
        <code>{formattedConfig}</code>
      </pre>
    </div>
  );
};
