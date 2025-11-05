import JSZip from 'jszip';

export async function extractQMDsFromZip(file: File): Promise<File[]> {
  const zip = await JSZip.loadAsync(file);
  const qmdFiles: File[] = [];

  for (const [filename, zipEntry] of Object.entries(zip.files)) {
    if (!zipEntry.dir && filename.toLowerCase().endsWith('.qmd')) {
      const blob = await zipEntry.async('blob');
      const newFile = new File([blob], filename.split('/').pop() || filename, {
        type: 'text/plain'
      });
      qmdFiles.push(newFile);
    }
  }

  return qmdFiles;
}

export async function extractQMDsFromFolder(
  entry: FileSystemEntry
): Promise<File[]> {
  const qmdFiles: File[] = [];

  async function traverseDirectory(dirEntry: FileSystemDirectoryEntry) {
    const reader = dirEntry.createReader();
    const entries = await new Promise<FileSystemEntry[]>((resolve) => {
      reader.readEntries(resolve);
    });

    for (const entry of entries) {
      if (entry.isFile) {
        const fileEntry = entry as FileSystemFileEntry;
        if (fileEntry.name.toLowerCase().endsWith('.qmd')) {
          const file = await new Promise<File>((resolve) => {
            fileEntry.file(resolve);
          });
          qmdFiles.push(file);
        }
      } else if (entry.isDirectory) {
        await traverseDirectory(entry as FileSystemDirectoryEntry);
      }
    }
  }

  if (entry.isDirectory) {
    await traverseDirectory(entry as FileSystemDirectoryEntry);
  } else if (entry.isFile && entry.name.toLowerCase().endsWith('.qmd')) {
    const fileEntry = entry as FileSystemFileEntry;
    const file = await new Promise<File>((resolve) => {
      fileEntry.file(resolve);
    });
    qmdFiles.push(file);
  }

  return qmdFiles;
}

export function sortFilesLexicographically(files: File[]): File[] {
  return [...files].sort((a, b) => a.name.localeCompare(b.name));
}
