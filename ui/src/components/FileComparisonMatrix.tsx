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

function parseVersion(version: string): number[] {
  return version.split('.').map(Number);
}

function isVersionInRange(version: string, min: string | null, max: string | null): boolean {
  const versionParts = parseVersion(version);

  if (min) {
    const minParts = parseVersion(min);
    for (let i = 0; i < Math.max(versionParts.length, minParts.length); i++) {
      const vVal = versionParts[i] || 0;
      const minVal = minParts[i] || 0;
      if (vVal < minVal) return false;
      if (vVal > minVal) break;
    }
  }

  if (max) {
    const maxParts = parseVersion(max);
    for (let i = 0; i < Math.max(versionParts.length, maxParts.length); i++) {
      const vVal = versionParts[i] || 0;
      const maxVal = maxParts[i] || 0;
      if (vVal > maxVal) return false;
      if (vVal < maxVal) break;
    }
  }

  return true;
}

interface FileComparisonMatrixProps {
  results: Map<string, CompareResponse>;
  filenames: string[];
  onRowClick: (filename: string) => void;
  filterDevices?: string[];
  filterMinVersion?: string | null;
  filterMaxVersion?: string | null;
}

export function FileComparisonMatrix({
  results,
  filenames,
  onRowClick,
  filterDevices = ['rm1', 'rm2', 'rmpp', 'rmppm'],
  filterMinVersion = null,
  filterMaxVersion = null
}: FileComparisonMatrixProps) {
  const deviceKeys = ['rm1', 'rm2', 'rmpp', 'rmppm'].filter(d => filterDevices.includes(d));

  const aggregateFileDeviceResults = (filename: string, device: string): 'all-compatible' | 'all-incompatible' | 'mixed' | 'no-data' => {
    const fileResults = results.get(filename);
    if (!fileResults) return 'no-data';

    let allResults = [...(fileResults.compatible || []), ...(fileResults.incompatible || [])];

    allResults = allResults.filter(r =>
      r.device === device &&
      isVersionInRange(r.os_version, filterMinVersion, filterMaxVersion)
    );

    if (allResults.length === 0) return 'no-data';

    const hasCompatible = allResults.some(r => r.compatible);
    const hasIncompatible = allResults.some(r => !r.compatible);

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
