import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { CompatibilityMatrix, type CompareResponse } from './CompatibilityMatrix';
import { DependencyResults } from './DependencyResults';

interface FileDetailModalProps {
  filename: string | null;
  results: CompareResponse | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FileDetailModal({ filename, results, open, onOpenChange }: FileDetailModalProps) {
  if (!filename || !results) return null;

  const allResults = [...results.compatible, ...results.incompatible];
  const resultsWithDependencies = allResults.filter(r => r.dependency_results && Object.keys(r.dependency_results).length > 0);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-6xl sm:max-w-6xl max-h-[80vh] overflow-y-auto"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="truncate">{filename}</DialogTitle>
        </DialogHeader>

        {resultsWithDependencies.length > 0 ? (
          <Tabs defaultValue="matrix" className="w-full">
            <TabsList>
              <TabsTrigger value="matrix">Compatibility Matrix</TabsTrigger>
              <TabsTrigger value="dependencies">Dependencies</TabsTrigger>
            </TabsList>
            <TabsContent value="matrix">
              <CompatibilityMatrix results={results} />
            </TabsContent>
            <TabsContent value="dependencies" className="space-y-6">
              {resultsWithDependencies.map(result => (
                <DependencyResults
                  key={`${result.os_version}-${result.device}`}
                  dependencyResults={result.dependency_results}
                  osVersion={result.os_version}
                  device={result.device}
                />
              ))}
            </TabsContent>
          </Tabs>
        ) : (
          <CompatibilityMatrix results={results} />
        )}
      </DialogContent>
    </Dialog>
  );
}
