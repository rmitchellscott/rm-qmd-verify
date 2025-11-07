import { useState } from 'react';
import { CheckCircle2, XCircle, ChevronRight, ChevronDown, AlertCircle } from 'lucide-react';
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table';
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip';
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover';

export interface MissingHashInfo {
  hash: string;
  line: number;
  column: number;
}

export interface ComparisonResult {
  hashtable: string;
  os_version: string;
  device: string;
  compatible: boolean;
  error_detail?: string;
  missing_hashes?: MissingHashInfo[];
}

export interface CompareResponse {
  compatible: ComparisonResult[];
  incompatible: ComparisonResult[];
  total_checked: number;
}

const deviceNames: Record<string, { short: string; full: string }> = {
  'rm1': { short: 'rM1', full: 'reMarkable 1' },
  'rm2': { short: 'rM2', full: 'reMarkable 2' },
  'rmpp': { short: 'rMPP', full: 'Paper Pro' },
  'rmppm': { short: 'rMPPM', full: 'Paper Pro Move' },
};

interface VersionInfo {
  full: string;
  majorMinorPatch: string;
  build: string | null;
  parts: number[];
}

function parseVersion(version: string): VersionInfo {
  const parts = version.split('.').map(p => parseInt(p, 10));
  const build = parts.length > 3 ? parts[3].toString() : null;
  const majorMinorPatch = parts.slice(0, 3).join('.');

  return {
    full: version,
    majorMinorPatch,
    build,
    parts
  };
}

function compareVersions(a: string, b: string): number {
  const aParts = parseVersion(a).parts;
  const bParts = parseVersion(b).parts;

  for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
    const aVal = aParts[i] || 0;
    const bVal = bParts[i] || 0;
    if (aVal !== bVal) {
      return bVal - aVal;
    }
  }
  return 0;
}

interface CompatibilityMatrixProps {
  results: CompareResponse;
}

export function CompatibilityMatrix({ results }: CompatibilityMatrixProps) {
  const [expandedVersions, setExpandedVersions] = useState<Set<string>>(new Set());

  const toggleVersionExpansion = (majorMinorPatch: string) => {
    setExpandedVersions(prev => {
      const next = new Set(prev);
      if (next.has(majorMinorPatch)) {
        next.delete(majorMinorPatch);
      } else {
        next.add(majorMinorPatch);
      }
      return next;
    });
  };

  const buildCompatibilityMatrix = () => {
    const allResults = [...results.compatible, ...results.incompatible];
    const deviceKeys = ['rm1', 'rm2', 'rmpp', 'rmppm'];

    const matrix: Record<string, Record<string, ComparisonResult | null>> = {};
    allResults.forEach(result => {
      if (!matrix[result.os_version]) {
        matrix[result.os_version] = {};
      }
      matrix[result.os_version][result.device] = result;
    });

    const versionsByGroup = new Map<string, string[]>();
    allResults.forEach(result => {
      const versionInfo = parseVersion(result.os_version);
      const group = versionInfo.majorMinorPatch;
      if (!versionsByGroup.has(group)) {
        versionsByGroup.set(group, []);
      }
      const versions = versionsByGroup.get(group)!;
      if (!versions.includes(result.os_version)) {
        versions.push(result.os_version);
      }
    });

    versionsByGroup.forEach((versions, _group) => {
      versions.sort(compareVersions);
    });

    const sortedGroups = Array.from(versionsByGroup.keys()).sort((a, b) => {
      const aMaxVersion = versionsByGroup.get(a)![0];
      const bMaxVersion = versionsByGroup.get(b)![0];
      return compareVersions(aMaxVersion, bMaxVersion);
    });

    const versionGroups = sortedGroups.map(group => ({
      majorMinorPatch: group,
      versions: versionsByGroup.get(group)!,
      hasMultipleBuilds: versionsByGroup.get(group)!.length > 1
    }));

    return { versionGroups, deviceKeys, matrix };
  };

  const { versionGroups, deviceKeys, matrix } = buildCompatibilityMatrix();

  const renderCompatibilityCell = (result: ComparisonResult | null | undefined, device: string) => (
    <TableCell key={device} className="text-center">
      {result?.compatible === true && (
        <Tooltip>
          <TooltipTrigger>
            <CheckCircle2 className="h-5 w-5 text-green-600 inline-block" />
          </TooltipTrigger>
          <TooltipContent>Compatible</TooltipContent>
        </Tooltip>
      )}
      {result?.compatible === false && (
        <Popover>
          <Tooltip>
            <TooltipTrigger asChild>
              <PopoverTrigger asChild>
                <button
                  className="cursor-pointer border-none bg-transparent p-0"
                  onClick={(e) => e.stopPropagation()}
                >
                  <XCircle className="h-5 w-5 text-red-600" />
                </button>
              </PopoverTrigger>
            </TooltipTrigger>
            <TooltipContent>Click for details</TooltipContent>
          </Tooltip>
          <PopoverContent>
            <div className="text-sm">
              {result.missing_hashes && result.missing_hashes.length > 0 && (
                <div className="mb-2 font-semibold">Missing {result.missing_hashes.length > 1 ? 'Hashes' : 'Hash'}</div>
              )}
              {result.missing_hashes && (() => {
                const maxPositionWidth = Math.max(...result.missing_hashes.map(h =>
                  `L${h.line}:C${h.column}`.length
                ));
                return result.missing_hashes.map(hashInfo => {
                  const position = `L${hashInfo.line}:C${hashInfo.column}`.padEnd(maxPositionWidth, ' ');
                  return (
                    <div key={hashInfo.hash} className="font-mono mb-1 text-sm break-all">
                      {position} {hashInfo.hash}
                    </div>
                  );
                });
              })()}
              {(!result.missing_hashes || result.missing_hashes.length === 0) && (
                <div className="font-bold">{result.error_detail || 'Unknown'}</div>
              )}
            </div>
          </PopoverContent>
        </Popover>
      )}
      {!result && <span className="text-muted-foreground">—</span>}
    </TableCell>
  );

  return (
    <TooltipProvider disableHoverableContent>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead className="min-w-24 sm:min-w-32"></TableHead>
            {deviceKeys.map(device => (
              <TableHead key={device} className="text-center">
                <span className="sm:hidden">{deviceNames[device].short}</span>
                <span className="hidden sm:inline">{deviceNames[device].full}</span>
              </TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {versionGroups.map(group => {
            const isExpanded = expandedVersions.has(group.majorMinorPatch);

            if (group.hasMultipleBuilds) {
              return (
                <>
                  <TableRow
                    key={group.majorMinorPatch}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() => toggleVersionExpansion(group.majorMinorPatch)}
                  >
                    <TableCell className="font-medium">
                      <div className="flex items-center gap-2">
                        <span>{group.majorMinorPatch}</span>
                        {isExpanded ? (
                          <ChevronDown className="h-4 w-4" />
                        ) : (
                          <ChevronRight className="h-4 w-4" />
                        )}
                      </div>
                    </TableCell>
                    {isExpanded ? (
                      deviceKeys.map(device => (
                        <TableCell key={device} className="text-center"></TableCell>
                      ))
                    ) : (
                      deviceKeys.map(device => {
                        const allResults = group.versions.map(v => matrix[v]?.[device]).filter(Boolean);

                        if (allResults.length === 0) {
                          return <TableCell key={device} className="text-center">
                            <span className="text-muted-foreground">—</span>
                          </TableCell>;
                        }

                        const hasFailure = allResults.some(r => r?.compatible === false);
                        const hasSuccess = allResults.some(r => r?.compatible === true);

                        if (hasFailure && hasSuccess) {
                          return (
                            <TableCell key={device} className="text-center">
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <button
                                    className="cursor-pointer border-none bg-transparent p-0"
                                    onClick={(e) => {
                                      e.stopPropagation();
                                      toggleVersionExpansion(group.majorMinorPatch);
                                    }}
                                  >
                                    <AlertCircle className="h-5 w-5 text-yellow-600 inline-block" />
                                  </button>
                                </TooltipTrigger>
                                <TooltipContent>Mixed results - click to see details</TooltipContent>
                              </Tooltip>
                            </TableCell>
                          );
                        } else if (hasFailure) {
                          const failureResult = allResults.find(r => r?.compatible === false);
                          return renderCompatibilityCell(failureResult!, device);
                        } else {
                          const successResult = allResults.find(r => r?.compatible === true);
                          return renderCompatibilityCell(successResult!, device);
                        }
                      })
                    )}
                  </TableRow>
                  {isExpanded && group.versions.map(version => (
                    <TableRow key={version} className="bg-muted/20">
                      <TableCell className="font-medium pl-10">{version}</TableCell>
                      {deviceKeys.map(device => {
                        const result = matrix[version]?.[device];
                        return renderCompatibilityCell(result, device);
                      })}
                    </TableRow>
                  ))}
                </>
              );
            } else {
              const version = group.versions[0];
              return (
                <TableRow key={version}>
                  <TableCell className="font-medium">{version}</TableCell>
                  {deviceKeys.map(device => {
                    const result = matrix[version]?.[device];
                    return renderCompatibilityCell(result, device);
                  })}
                </TableRow>
              );
            }
          })}
        </TableBody>
      </Table>
    </TooltipProvider>
  );
}
