import { useState, useEffect } from 'react';
import { useNavigate, useLocation } from 'react-router-dom';
import { FileComparisonMatrix } from '@/components/FileComparisonMatrix';
import { FileDetailModal } from '@/components/FileDetailModal';
import ThemeSwitcher from '@/components/ThemeSwitcher';
import type { CompareResponse } from '@/components/CompatibilityMatrix';

interface LocationState {
  results: Record<string, CompareResponse>;
  filenames: string[];
}

export function ComparisonResultsPage() {
  const navigate = useNavigate();
  const location = useLocation();
  const state = location.state as LocationState | null;
  const [selectedFileForModal, setSelectedFileForModal] = useState<string | null>(null);
  const [versionInfo, setVersionInfo] = useState<{ version: string } | null>(null);

  useEffect(() => {
    const fetchVersionInfo = async () => {
      try {
        const response = await fetch('/api/version');
        if (response.ok) {
          const data = await response.json();
          setVersionInfo(data);
        }
      } catch (error) {
        console.error('Failed to fetch version info:', error);
      }
    };

    fetchVersionInfo();
  }, []);

  if (!state || !state.results || !state.filenames || Object.keys(state.results).length === 0) {
    navigate('/');
    return null;
  }

  const resultsMap = new Map(Object.entries(state.results));

  return (
    <div className="min-h-screen flex flex-col">
      <header className="flex items-center justify-between px-8 py-2 bg-background">
        <h1 className="text-2xl font-bold">reMarkable QMD Verifier</h1>
        <ThemeSwitcher />
      </header>
      <div className="sticky top-0 z-40 border-b bg-background">
        <div className="container mx-auto px-4 py-4 space-y-2">
          <div>
            <a
              onClick={() => navigate('/')}
              className="text-sm text-muted-foreground hover:text-foreground hover:underline cursor-pointer"
            >
              ← Start Over
            </a>
          </div>
          <div>
            <h1 className="text-2xl font-semibold">File Comparison Results</h1>
          </div>
        </div>
      </div>

      <div className="container mx-auto px-4 py-6 flex-1">
        <div className="max-w-4xl mx-auto">
          <FileComparisonMatrix
            results={resultsMap}
            filenames={state.filenames}
            onRowClick={setSelectedFileForModal}
          />
        </div>
      </div>

      {versionInfo && (
        <footer className="py-2 bg-background">
          <div className="text-center text-sm text-muted-foreground">
            <span>{versionInfo.version} • </span>
            <a
              href="https://github.com/rmitchellscott/rm-qmd-verify"
              target="_blank"
              rel="noopener noreferrer"
              className="text-muted-foreground hover:underline"
            >
              GitHub
            </a>
          </div>
        </footer>
      )}

      <FileDetailModal
        filename={selectedFileForModal}
        results={selectedFileForModal ? resultsMap.get(selectedFileForModal) : undefined}
        open={!!selectedFileForModal}
        onOpenChange={(open) => !open && setSelectedFileForModal(null)}
      />
    </div>
  );
}
