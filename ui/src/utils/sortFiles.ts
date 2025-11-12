import type { CompareResponse } from '@/components/CompatibilityMatrix';
import type { SortMethod } from '@/hooks/useSortPreferences';

export interface FileTreeNode {
  filename: string;
  isRoot: boolean;
  position?: number;
  children: FileTreeNode[];
}

export type FileSortResult = string[] | FileTreeNode[];

function isRootFile(filename: string): boolean {
  return !filename.includes('/') && !filename.includes('\\');
}

function buildDependencyTree(
  filenames: string[],
  results: Map<string, CompareResponse>
): FileTreeNode[] {
  const rootFiles = filenames.filter(isRootFile);
  const treeNodes: FileTreeNode[] = [];

  for (const rootFilename of rootFiles) {
    const result = results.get(rootFilename);
    if (!result) continue;

    const dependencyMap = new Map<string, number>();
    const allResults = [...result.compatible, ...result.incompatible];

    allResults.forEach(compResult => {
      if (compResult.dependency_results) {
        Object.entries(compResult.dependency_results).forEach(([depPath, depResult]) => {
          if (depResult.position !== undefined && depResult.position >= 0) {
            const existingPos = dependencyMap.get(depPath);
            if (existingPos === undefined || depResult.position < existingPos) {
              dependencyMap.set(depPath, depResult.position);
            }
          }
        });
      }
    });

    const children: FileTreeNode[] = Array.from(dependencyMap.entries())
      .sort((a, b) => a[1] - b[1])
      .map(([depPath, position]) => ({
        filename: depPath,
        isRoot: false,
        position,
        children: [],
      }));

    treeNodes.push({
      filename: rootFilename,
      isRoot: true,
      children,
    });
  }

  return treeNodes;
}

function hasFailures(
  result: CompareResponse,
  filterDevices: string[],
  filterMinVersion: string | null,
  filterMaxVersion: string | null
): boolean {
  const isVersionInRange = (version: string): boolean => {
    if (filterMinVersion && version < filterMinVersion) return false;
    if (filterMaxVersion && version > filterMaxVersion) return false;
    return true;
  };

  return result.incompatible.some(r => {
    const deviceMatch = filterDevices.includes(r.device);
    const versionMatch = isVersionInRange(r.os_version);
    return deviceMatch && versionMatch;
  });
}

export function sortFiles(
  filenames: string[],
  results: Map<string, CompareResponse>,
  sortMethod: SortMethod,
  filterDevices: string[],
  filterMinVersion: string | null,
  filterMaxVersion: string | null
): FileSortResult {
  switch (sortMethod) {
    case 'dependency': {
      return buildDependencyTree(filenames, results);
    }

    case 'failures': {
      return [...filenames].sort((a, b) => {
        const resultA = results.get(a);
        const resultB = results.get(b);

        if (!resultA || !resultB) return 0;

        const hasFailureA = hasFailures(resultA, filterDevices, filterMinVersion, filterMaxVersion);
        const hasFailureB = hasFailures(resultB, filterDevices, filterMinVersion, filterMaxVersion);

        if (hasFailureA && !hasFailureB) return -1;
        if (!hasFailureA && hasFailureB) return 1;

        return a.localeCompare(b);
      });
    }

    case 'alphabetical': {
      return [...filenames].sort((a, b) => a.localeCompare(b));
    }

    default:
      return filenames;
  }
}

export function detectHasDependencies(results: Map<string, CompareResponse>): boolean {
  for (const result of results.values()) {
    const allResults = [...result.compatible, ...result.incompatible];
    for (const compResult of allResults) {
      if (compResult.dependency_results && Object.keys(compResult.dependency_results).length > 0) {
        return true;
      }
    }
  }
  return false;
}
