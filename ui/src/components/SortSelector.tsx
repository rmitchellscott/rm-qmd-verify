import { Label } from '@/components/ui/label';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import type { SortMethod } from '@/hooks/useSortPreferences';

interface SortSelectorProps {
  sortMethod: SortMethod;
  onChange: (method: SortMethod) => void;
}

const SORT_OPTIONS: Record<SortMethod, string> = {
  dependency: 'Dependency Order',
  failures: 'Failures First',
  alphabetical: 'Alphabetical',
};

export function SortSelector({ sortMethod, onChange }: SortSelectorProps) {
  return (
    <div className="flex items-center gap-3">
      <Label htmlFor="sort-select" className="text-sm font-medium whitespace-nowrap">
        Sort by
      </Label>
      <Select value={sortMethod} onValueChange={(value) => onChange(value as SortMethod)}>
        <SelectTrigger id="sort-select" size="sm">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(Object.keys(SORT_OPTIONS) as SortMethod[]).map((method) => (
            <SelectItem key={method} value={method}>
              {SORT_OPTIONS[method]}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
