import { useMemo } from 'react';
import { Label } from '@/components/ui/label';
import { DualRangeSlider } from '@/components/ui/dual-range-slider';

interface VersionRangeSliderProps {
  availableVersions: string[];
  minVersion: string | null;
  maxVersion: string | null;
  onChange: (min: string | null, max: string | null) => void;
}

function compareVersions(a: string, b: string): number {
  const aParts = a.split('.').map(Number);
  const bParts = b.split('.').map(Number);

  for (let i = 0; i < Math.max(aParts.length, bParts.length); i++) {
    const aVal = aParts[i] || 0;
    const bVal = bParts[i] || 0;
    if (aVal !== bVal) {
      return aVal - bVal;
    }
  }
  return 0;
}

export function VersionRangeSlider({
  availableVersions,
  minVersion,
  maxVersion,
  onChange,
}: VersionRangeSliderProps) {
  const sortedVersions = useMemo(() => {
    return [...availableVersions].sort(compareVersions);
  }, [availableVersions]);

  const minIndex = useMemo(() => {
    if (!minVersion) return 0;
    const idx = sortedVersions.indexOf(minVersion);
    return idx === -1 ? 0 : idx;
  }, [minVersion, sortedVersions]);

  const maxIndex = useMemo(() => {
    if (!maxVersion) return sortedVersions.length - 1;
    const idx = sortedVersions.indexOf(maxVersion);
    return idx === -1 ? sortedVersions.length - 1 : idx;
  }, [maxVersion, sortedVersions]);

  const handleChange = (values: number[]) => {
    const [newMinIdx, newMaxIdx] = values;
    const newMin = sortedVersions[newMinIdx];
    const newMax = sortedVersions[newMaxIdx];

    onChange(
      newMinIdx === 0 ? null : newMin,
      newMaxIdx === sortedVersions.length - 1 ? null : newMax
    );
  };

  if (sortedVersions.length === 0) {
    return null;
  }

  return (
    <div>
      <Label className="text-sm font-medium">Version Range</Label>
      <div className="space-y-4 mt-3 pt-2 pb-2 px-4">
        <DualRangeSlider
          min={0}
          max={sortedVersions.length - 1}
          step={1}
          value={[minIndex, maxIndex]}
          onValueChange={handleChange}
          className="w-full"
          label={(value) => (
            <span className="text-xs font-medium">
              {sortedVersions[value ?? 0]}
            </span>
          )}
          labelPosition="bottom"
        />
      </div>
    </div>
  );
}
