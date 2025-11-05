import { CheckCircle2, XCircle, AlertCircle } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import type { CompareResponse } from './CompatibilityMatrix';

const deviceNames: Record<string, { short: string; full: string }> = {
  'rm1': { short: 'rM1', full: 'reMarkable 1' },
  'rm2': { short: 'rM2', full: 'reMarkable 2' },
  'rmpp': { short: 'rMPP', full: 'Paper Pro' },
  'rmppm': { short: 'rMPPM', full: 'Paper Pro Move' },
};

interface FileComparisonMatrixProps {
  results: Map<string, CompareResponse>;
  filenames: string[];
  onRowClick: (filename: string) => void;
}

export function FileComparisonMatrix({ results, filenames, onRowClick }: FileComparisonMatrixProps) {
  const deviceKeys = ['rm1', 'rm2', 'rmpp', 'rmppm'];

  const aggregateFileDeviceResults = (filename: string, device: string): 'all-compatible' | 'all-incompatible' | 'mixed' | 'no-data' => {
    const fileResults = results.get(filename);
    if (!fileResults) return 'no-data';

    const allResults = [...fileResults.compatible, ...fileResults.incompatible];
    const deviceResults = allResults.filter(r => r.device === device);

    if (deviceResults.length === 0) return 'no-data';

    const hasCompatible = deviceResults.some(r => r.compatible);
    const hasIncompatible = deviceResults.some(r => !r.compatible);

    if (hasCompatible && hasIncompatible) return 'mixed';
    if (hasCompatible) return 'all-compatible';
    if (hasIncompatible) return 'all-incompatible';
    return 'no-data';
  };

  const renderDeviceCell = (filename: string, device: string) => {
    const status = aggregateFileDeviceResults(filename, device);

    return (
      <TableCell key={device} className="text-center">
        {status === 'all-compatible' && (
          <Tooltip>
            <TooltipTrigger>
              <CheckCircle2 className="h-5 w-5 text-green-600 inline-block" />
            </TooltipTrigger>
            <TooltipContent>All versions compatible</TooltipContent>
          </Tooltip>
        )}
        {status === 'all-incompatible' && (
          <Tooltip>
            <TooltipTrigger>
              <XCircle className="h-5 w-5 text-red-600 inline-block" />
            </TooltipTrigger>
            <TooltipContent>All versions incompatible</TooltipContent>
          </Tooltip>
        )}
        {status === 'mixed' && (
          <Tooltip>
            <TooltipTrigger>
              <AlertCircle className="h-5 w-5 text-yellow-600 inline-block" />
            </TooltipTrigger>
            <TooltipContent>Mixed results across versions</TooltipContent>
          </Tooltip>
        )}
        {status === 'no-data' && (
          <span className="text-muted-foreground">—</span>
        )}
      </TableCell>
    );
  };

  return (
    <TooltipProvider disableHoverableContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-48">File</TableHead>
            {deviceKeys.map(device => (
              <TableHead key={device} className="text-center">
                <span className="sm:hidden">{deviceNames[device].short}</span>
                <span className="hidden sm:inline">{deviceNames[device].full}</span>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {filenames.map(filename => (
            <TableRow
              key={filename}
              className="cursor-pointer hover:bg-muted/50"
              onClick={() => onRowClick(filename)}
            >
              <TableCell className="font-medium">
                <Tooltip>
                  <TooltipTrigger asChild>
                    <span className="truncate block max-w-xs">{filename}</span>
                  </TooltipTrigger>
                  <TooltipContent>Click to see detailed version compatibility</TooltipContent>
                </Tooltip>
              </TableCell>
              {deviceKeys.map(device => renderDeviceCell(filename, device))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </TooltipProvider>
  );
}
