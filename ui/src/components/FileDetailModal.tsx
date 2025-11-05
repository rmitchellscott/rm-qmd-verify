import { Dialog, DialogContent, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { CompatibilityMatrix, type CompareResponse } from './CompatibilityMatrix';
import { Card, CardContent } from '@/components/ui/card';

interface FileDetailModalProps {
  filename: string | null;
  results: CompareResponse | undefined;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

export function FileDetailModal({ filename, results, open, onOpenChange }: FileDetailModalProps) {
  if (!filename || !results) return null;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-w-6xl sm:max-w-6xl max-h-[80vh] overflow-y-auto"
        onOpenAutoFocus={(e) => e.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle className="truncate">{filename}</DialogTitle>
        </DialogHeader>
        <Card>
          <CardContent className="pt-6">
            <CompatibilityMatrix results={results} />
          </CardContent>
        </Card>
      </DialogContent>
    </Dialog>
  );
}
