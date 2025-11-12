import { Label } from '@/components/ui/label';
import { Skeleton } from '@/components/ui/skeleton';

export function VersionRangeSliderSkeleton() {
  return (
    <div>
      <Label className="text-sm font-medium">Version Range</Label>
      <div className="space-y-4 mt-3 pt-2 pb-2 px-4">
        <Skeleton className="h-2 w-full" />
      </div>
    </div>
  );
}
