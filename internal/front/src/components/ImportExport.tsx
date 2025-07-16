import React, { useState, useRef } from 'react';
import { Download, Upload, FileText, AlertCircle, CheckCircle } from 'lucide-react';
import { NetworkConfig } from '../types/config';
import { exportConfig, importConfig, downloadFile } from '../utils/config';
import { usePortal } from '../hooks/usePortal';

interface ImportExportProps {
  config: NetworkConfig;
  onImport: (config: NetworkConfig) => void;
}

export const ImportExport: React.FC<ImportExportProps> = ({ config, onImport }) => {
  const [importText, setImportText] = useState('');
  const [isImportModalOpen, setIsImportModalOpen] = useState(false);
  const [importError, setImportError] = useState<string | null>(null);
  const [importSuccess, setImportSuccess] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const handleExport = () => {
    const configJson = exportConfig(config);
    const timestamp = new Date().toISOString().slice(0, 19).replace(/:/g, '-');
    downloadFile(configJson, `network-config-${timestamp}.json`);
  };

  const handleImportFromText = () => {
    try {
      const importedConfig = importConfig(importText);
      onImport(importedConfig);
      setImportSuccess(true);
      setImportError(null);
      setTimeout(() => {
        setIsImportModalOpen(false);
        setImportText('');
        setImportSuccess(false);
      }, 1500);
    } catch (error) {
      setImportError(error instanceof Error ? error.message : 'Failed to import configuration');
      setImportSuccess(false);
    }
  };

  const handleFileImport = (event: React.ChangeEvent<HTMLInputElement>) => {
    const file = event.target.files?.[0];
    if (!file) return;

    const reader = new FileReader();
    reader.onload = (e) => {
      const content = e.target?.result as string;
      setImportText(content);
    };
    reader.readAsText(file);
  };

  const resetImport = () => {
    setImportText('');
    setImportError(null);
    setImportSuccess(false);
  };
  const renderModal = usePortal('modal-root');
  return (
    <>
      <div className="flex items-center gap-3">
        <button
          onClick={handleExport}
          className="inline-flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-emerald-500 to-green-600 text-white hover:from-emerald-600 hover:to-green-700 rounded-lg transition-all duration-300 shadow-lg hover:shadow-emerald-500/25"
        >
          <Download className="w-4 h-4" />
          Export Config
        </button>

        <button
          onClick={() => setIsImportModalOpen(true)}
          className="inline-flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-blue-500 to-cyan-600 text-white hover:from-blue-600 hover:to-cyan-700 rounded-lg transition-all duration-300 shadow-lg hover:shadow-blue-500/25"
        >
          <Upload className="w-4 h-4" />
          Import Config
        </button>
      </div>

      {/* Import Modal */}
      {isImportModalOpen && renderModal(
        <div className="fixed inset-0 bg-black/70  flex items-center justify-center p-4 z-50">
          <div className="bg-gray-800/90 backdrop-blur-xl rounded-xl shadow-2xl border border-gray-700/50 max-w-2xl w-full max-h-[90vh] overflow-auto">
            <div className="p-6 border-b border-gray-700/50">
              <h2 className="text-xl font-semibold text-white">Import Configuration</h2>
            </div>

            <div className="p-6 space-y-6">
              {/* File Upload */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Upload from File
                </label>
                <div className="flex items-center gap-3">
                  <input
                    ref={fileInputRef}
                    type="file"
                    accept=".json,.txt"
                    onChange={handleFileImport}
                    className="hidden"
                  />
                  <button
                    onClick={() => fileInputRef.current?.click()}
                    className="inline-flex items-center gap-2 px-4 py-2 border border-gray-600 hover:border-gray-500 rounded-lg transition-all duration-300 text-gray-300 hover:bg-gray-700/30 "
                  >
                    <FileText className="w-4 h-4" />
                    Choose File
                  </button>
                  <span className="text-sm text-gray-400">
                    Select a JSON configuration file
                  </span>
                </div>
              </div>

              <div className="relative">
                <div className="absolute inset-0 flex items-center">
                  <div className="w-full border-t border-gray-600" />
                </div>
                <div className="relative flex justify-center text-sm">
                  <span className="px-2 bg-gray-800 text-gray-400">or paste JSON</span>
                </div>
              </div>

              {/* Text Import */}
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">
                  Paste Configuration JSON
                </label>
                <textarea
                  value={importText}
                  onChange={(e) => {
                    setImportText(e.target.value);
                    setImportError(null);
                    setImportSuccess(false);
                  }}
                  placeholder="Paste your configuration JSON here..."
                  rows={10}
                  className={`w-full px-3 py-2 bg-gray-700/50 border rounded-lg focus:ring-2 focus:ring-pink-500 focus:border-transparent font-mono text-sm text-white placeholder-gray-400  transition-all duration-300 ${importError ? 'border-red-500/50' : 'border-gray-600'
                    }`}
                />
              </div>

              {/* Status Messages */}
              {importError && (
                <div className="flex items-center gap-2 p-3 bg-red-500/10 border border-red-500/20 rounded-lg ">
                  <AlertCircle className="w-5 h-5 text-red-400 flex-shrink-0" />
                  <span className="text-sm text-red-300">{importError}</span>
                </div>
              )}

              {importSuccess && (
                <div className="flex items-center gap-2 p-3 bg-green-500/10 border border-green-500/20 rounded-lg ">
                  <CheckCircle className="w-5 h-5 text-green-400 flex-shrink-0" />
                  <span className="text-sm text-green-300">Configuration imported successfully!</span>
                </div>
              )}
            </div>

            {/* Actions */}
            <div className="p-6 border-t border-gray-700/50 flex justify-end gap-3">
              <button
                onClick={() => {
                  setIsImportModalOpen(false);
                  resetImport();
                }}
                className="px-4 py-2 text-gray-300 bg-gray-700/50 hover:bg-gray-600/50 rounded-lg transition-all duration-300 "
              >
                Cancel
              </button>
              <button
                onClick={handleImportFromText}
                disabled={!importText.trim() || importSuccess}
                className="px-4 py-2 bg-gradient-to-r from-pink-500 to-purple-600 text-white hover:from-pink-600 hover:to-purple-700 disabled:from-gray-600 disabled:to-gray-700 disabled:cursor-not-allowed rounded-lg transition-all duration-300 shadow-lg hover:shadow-pink-500/25"
              >
                Import Configuration
              </button>
            </div>
          </div>
        </div>
      )}
    </>
  );
};