import { CheckCircle2, XCircle, CircleMinus } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { ValidationResult } from './CompatibilityMatrix';

interface DependencyResultsProps {
  dependencyResults: Record<string, ValidationResult> | undefined;
  osVersion: string;
  device: string;
}

export function DependencyResults({ dependencyResults, osVersion, device }: DependencyResultsProps) {
  if (!dependencyResults || Object.keys(dependencyResults).length === 0) {
    return null;
  }

  const sortedEntries = Object.entries(dependencyResults).sort((a, b) => {
    const posA = a[1].position ?? 999;
    const posB = b[1].position ?? 999;
    return posA - posB;
  });

  return (
    <div className="mt-6">
      <h3 className="text-lg font-semibold mb-3">
        Dependency Validation Results ({osVersion} - {device})
      </h3>
      <TooltipProvider>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead className="w-16 text-center">Status</TableHead>
              <TableHead>File Path</TableHead>
              <TableHead className="w-24 text-center">Position</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {sortedEntries.map(([filePath, result]) => (
              <TableRow key={filePath}>
                <TableCell className="text-center">
                  {result.status === 'validated' && (
                    <Tooltip>
                      <TooltipTrigger>
                        <CheckCircle2 className="h-5 w-5 text-green-600 inline-block" />
                      </TooltipTrigger>
                      <TooltipContent>Validated successfully</TooltipContent>
                    </Tooltip>
                  )}
                  {result.status === 'failed' && (
                    <Tooltip>
                      <TooltipTrigger>
                        <XCircle className="h-5 w-5 text-red-600 inline-block" />
                      </TooltipTrigger>
                      <TooltipContent>
                        <div className="text-sm">
                          {result.hash_errors && result.hash_errors.length > 0 && (
                            <div>Hash errors: {result.hash_errors.length}</div>
                          )}
                          {result.process_errors && result.process_errors.length > 0 && (
                            <div>
                              {result.process_errors.map((err, i) => (
                                <div key={i}>{err}</div>
                              ))}
                            </div>
                          )}
                        </div>
                      </TooltipContent>
                    </Tooltip>
                  )}
                  {result.status === 'not_attempted' && (
                    <Tooltip>
                      <TooltipTrigger>
                        <CircleMinus className="h-5 w-5 text-gray-400 inline-block" />
                      </TooltipTrigger>
                      <TooltipContent>Not attempted due to prior failure</TooltipContent>
                    </Tooltip>
                  )}
                </TableCell>
                <TableCell className="font-mono text-sm">{filePath}</TableCell>
                <TableCell className="text-center text-muted-foreground">
                  {result.position !== undefined && result.position >= 0 ? result.position : '-'}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TooltipProvider>
    </div>
  );
}
