import React, { useState, useCallback } from 'react';
import { Plus, Network, Settings } from 'lucide-react';
import { NetworkConfig, EntryPoint } from './types/config';
import { createDefaultEntryPoint } from './utils/config';
import { EntryPointCard } from './components/EntryPointCard';
import { EntryPointForm } from './components/EntryPointForm';
import { JsonPreview } from './components/JsonPreview';
import { ImportExport } from './components/ImportExport';
import { DragDropList } from './components/DragDropList';

function App() {
  const [config, setConfig] = useState<NetworkConfig>({ entrypoints: [] });
  const [showForm, setShowForm] = useState(false);
  const [editingEntryPoint, setEditingEntryPoint] = useState<EntryPoint | null>(null);
  const [showJsonPreview, setShowJsonPreview] = useState(false);

  const handleAddEntryPoint = useCallback(() => {
    setEditingEntryPoint(createDefaultEntryPoint());
    setShowForm(true);
  }, []);

  const handleEditEntryPoint = useCallback((entryPoint: EntryPoint) => {
    setEditingEntryPoint(entryPoint);
    setShowForm(true);
  }, []);

  const handleSaveEntryPoint = useCallback((entryPoint: EntryPoint) => {
    setConfig(prevConfig => {
      const existingIndex = prevConfig.entrypoints.findIndex(ep => ep.id === entryPoint.id);
      
      if (existingIndex >= 0) {
        // Update existing
        const newEntrypoints = [...prevConfig.entrypoints];
        newEntrypoints[existingIndex] = entryPoint;
        return { entrypoints: newEntrypoints };
      } else {
        // Add new
        return { entrypoints: [...prevConfig.entrypoints, entryPoint] };
      }
    });
    
    setShowForm(false);
    setEditingEntryPoint(null);
  }, []);

  const handleDeleteEntryPoint = useCallback((id: string) => {
    if (window.confirm('Are you sure you want to delete this entry point?')) {
      setConfig(prevConfig => ({
        entrypoints: prevConfig.entrypoints.filter(ep => ep.id !== id)
      }));
    }
  }, []);

  const handleCancelForm = useCallback(() => {
    setShowForm(false);
    setEditingEntryPoint(null);
  }, []);

  const handleReorderEntryPoints = useCallback((reorderedEntryPoints: EntryPoint[]) => {
    setConfig({ entrypoints: reorderedEntryPoints });
  }, []);

  const handleImportConfig = useCallback((importedConfig: NetworkConfig) => {
    setConfig(importedConfig);
  }, []);

  return (
    <div className="min-h-screen bg-gradient-to-br from-gray-900 via-slate-900 to-black">
      {/* Header */}
      <header className="bg-gray-900/80 backdrop-blur-xl border-b border-gray-700/50">
        <div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
          <div className="flex items-center justify-between h-16">
            <div className="flex items-center gap-3">
              <div className="p-2 bg-gradient-to-r from-pink-500 to-purple-600 rounded-lg shadow-lg">
                <Network className="w-6 h-6 text-white" />
              </div>
              <div>
                <h1 className="text-xl font-semibold text-white">Network Configuration Editor</h1>
                <p className="text-sm text-gray-400">Manage your network entry points and routing</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3">
              <ImportExport config={config} onImport={handleImportConfig} />
              
              <button
                onClick={() => setShowJsonPreview(!showJsonPreview)}
                className={`inline-flex items-center gap-2 px-4 py-2 border rounded-lg transition-all duration-300 backdrop-blur-sm ${
                  showJsonPreview 
                    ? 'bg-gradient-to-r from-pink-500/20 to-purple-600/20 border-pink-500/30 text-pink-300' 
                    : 'border-gray-600 hover:border-gray-500 text-gray-300 hover:bg-gray-800/50'
                }`}
              >
                <Settings className="w-4 h-4" />
                {showJsonPreview ? 'Hide' : 'Show'} JSON
              </button>
            </div>
          </div>
        </div>
      </header>

      {/* Main Content */}
      <main className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8 py-8">
        <div className={`grid gap-8 ${showJsonPreview ? 'lg:grid-cols-2' : 'lg:grid-cols-1'}`}>
          {/* Entry Points Section */}
          <div className="space-y-6">
            <div className="flex items-center justify-between">
              <div>
                <h2 className="text-lg font-semibold text-white">Entry Points</h2>
                <p className="text-sm text-gray-400">
                  {config.entrypoints.length} entry point{config.entrypoints.length !== 1 ? 's' : ''} configured
                </p>
              </div>
              
              <button
                onClick={handleAddEntryPoint}
                className="inline-flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-pink-500 to-purple-600 text-white hover:from-pink-600 hover:to-purple-700 rounded-lg transition-all duration-300 shadow-lg hover:shadow-pink-500/25 backdrop-blur-sm"
              >
                <Plus className="w-4 h-4" />
                Add Entry Point
              </button>
            </div>

            {config.entrypoints.length === 0 ? (
              <div className="text-center py-12 bg-gray-800/50 backdrop-blur-sm rounded-lg border border-gray-700/50">
                <Network className="w-12 h-12 text-gray-500 mx-auto mb-4" />
                <h3 className="text-lg font-medium text-white mb-2">No Entry Points</h3>
                <p className="text-gray-400 mb-6">Get started by adding your first network entry point.</p>
                <button
                  onClick={handleAddEntryPoint}
                  className="inline-flex items-center gap-2 px-4 py-2 bg-gradient-to-r from-pink-500 to-purple-600 text-white hover:from-pink-600 hover:to-purple-700 rounded-lg transition-all duration-300 shadow-lg hover:shadow-pink-500/25"
                >
                  <Plus className="w-4 h-4" />
                  Add Entry Point
                </button>
              </div>
            ) : (
              <DragDropList
                items={config.entrypoints}
                onReorder={handleReorderEntryPoints}
                renderItem={(entryPoint) => (
                  <EntryPointCard
                    entryPoint={entryPoint}
                    onEdit={() => handleEditEntryPoint(entryPoint)}
                    onDelete={() => handleDeleteEntryPoint(entryPoint.id)}
                  />
                )}
              />
            )}
          </div>

          {/* JSON Preview Section */}
          {showJsonPreview && (
            <div className="space-y-6">
              <div>
                <h2 className="text-lg font-semibold text-white mb-4">Configuration Preview</h2>
                <JsonPreview config={config} />
              </div>
            </div>
          )}
        </div>
      </main>

      {/* Forms */}
      {showForm && editingEntryPoint && (
        <EntryPointForm
          entryPoint={editingEntryPoint}
          onSave={handleSaveEntryPoint}
          onCancel={handleCancelForm}
          isEditing={config.entrypoints.some(ep => ep.id === editingEntryPoint.id)}
        />
      )}
    </div>
  );
}

export default App;