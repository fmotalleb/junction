import React from 'react';
import { NetworkConfig } from '../types/config';
import { exportConfig } from '../utils/config';

interface JsonPreviewProps {
  config: NetworkConfig;
}

export const JsonPreview: React.FC<JsonPreviewProps> = ({ config }) => {
  const jsonString = exportConfig(config);

  return (
    <div className="bg-gray-900/80 backdrop-blur-xl rounded-lg border border-gray-700/50 p-4 overflow-auto max-h-96">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-sm font-medium text-gray-300">JSON Preview</h3>
        <div className="flex items-center gap-2">
          <div className="flex items-center gap-1">
            <div className="w-2 h-2 bg-gradient-to-r from-emerald-400 to-green-500 rounded-full shadow-lg shadow-emerald-500/50"></div>
            <span className="text-xs text-gray-400">Valid</span>
          </div>
        </div>
      </div>
      <pre className="text-sm text-gray-300 font-mono leading-relaxed">
        <code>{jsonString}</code>
      </pre>
    </div>
  );
};