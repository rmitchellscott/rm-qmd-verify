import { Button } from '@/components/ui/button';

interface FileListProps {
  files: File[];
  onRemove: (index: number) => void;
  onClearAll: () => void;
  disabled?: boolean;
}

export function FileList({ files, onRemove, onClearAll, disabled = false }: FileListProps) {
  if (files.length === 0) return null;

  return (
    <div className="space-y-4">
      <div className="flex justify-between items-center">
        <h3 className="text-sm font-medium">
          Files ({files.length})
        </h3>
        <Button
          variant="ghost"
          size="sm"
          onClick={onClearAll}
          disabled={disabled}
        >
          Clear All
        </Button>
      </div>
      <div className="space-y-2">
        {files.map((file, index) => (
          <div key={index} className="flex justify-between items-center">
            <p className="text-sm font-medium">{file.name}</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => onRemove(index)}
              disabled={disabled}
            >
              Remove
            </Button>
          </div>
        ))}
      </div>
    </div>
  );
}
