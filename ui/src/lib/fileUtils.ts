import JSZip from 'jszip';
import pako from 'pako';

export async function extractQMDsFromZip(file: File): Promise<File[]> {
  const zip = await JSZip.loadAsync(file);
  const qmdFiles: File[] = [];

  for (const [filename, zipEntry] of Object.entries(zip.files)) {
    if (!zipEntry.dir && filename.toLowerCase().endsWith('.qmd')) {
      const blob = await zipEntry.async('blob');
      // Preserve the full path in the filename to maintain folder structure
      const newFile = new File([blob], filename, {
        type: 'text/plain'
      });
      qmdFiles.push(newFile);
    }
  }

  return qmdFiles;
}

export async function extractQMDsFromTarGz(file: File): Promise<File[]> {
  const qmdFiles: File[] = [];

  // Read file as ArrayBuffer
  const arrayBuffer = await file.arrayBuffer();
  const uint8Array = new Uint8Array(arrayBuffer);

  // Decompress gzip
  const decompressed = pako.ungzip(uint8Array);

  // Parse TAR format manually (simple implementation)
  let offset = 0;
  while (offset < decompressed.length) {
    // Read TAR header (512 bytes)
    const header = decompressed.slice(offset, offset + 512);

    // Get filename (first 100 bytes, null-terminated)
    const nameBytes = header.slice(0, 100);
    const nameEnd = nameBytes.indexOf(0);
    const filename = new TextDecoder().decode(nameBytes.slice(0, nameEnd > 0 ? nameEnd : 100)).trim();

    // Get file size (bytes 124-135, octal string)
    const sizeBytes = header.slice(124, 136);
    const sizeStr = new TextDecoder().decode(sizeBytes).trim().replace(/\0/g, '');
    const fileSize = parseInt(sizeStr, 8) || 0;

    // Get file type (byte 156) - '0' or '\0' for regular file
    const fileType = String.fromCharCode(header[156]);

    offset += 512; // Move past header

    if (filename && fileSize > 0 && (fileType === '0' || fileType === '\0') && filename.toLowerCase().endsWith('.qmd')) {
      // Extract file content
      const fileData = decompressed.slice(offset, offset + fileSize);
      const blob = new Blob([fileData], { type: 'text/plain' });
      const qmdFile = new File([blob], filename, {
        type: 'text/plain'
      });
      qmdFiles.push(qmdFile);
    }

    // Move to next entry (file size rounded up to 512-byte boundary)
    offset += Math.ceil(fileSize / 512) * 512;

    // Check for end of archive (two consecutive zero blocks)
    if (offset + 1024 <= decompressed.length) {
      const nextBlock = decompressed.slice(offset, offset + 1024);
      if (nextBlock.every((byte: number) => byte === 0)) {
        break;
      }
    }
  }

  return qmdFiles;
}

export async function extractQMDsFromFolder(
  entry: FileSystemEntry
): Promise<File[]> {
  const qmdFiles: File[] = [];

  async function traverseDirectory(dirEntry: FileSystemDirectoryEntry, basePath: string = '') {
    const reader = dirEntry.createReader();
    const entries = await new Promise<FileSystemEntry[]>((resolve) => {
      reader.readEntries(resolve);
    });

    for (const entry of entries) {
      const relativePath = basePath ? `${basePath}/${entry.name}` : entry.name;

      if (entry.isFile) {
        const fileEntry = entry as FileSystemFileEntry;
        if (fileEntry.name.toLowerCase().endsWith('.qmd')) {
          const file = await new Promise<File>((resolve) => {
            fileEntry.file(resolve);
          });
          // Create new File with preserved path
          const fileWithPath = new File([file], relativePath, {
            type: 'text/plain'
          });
          qmdFiles.push(fileWithPath);
        }
      } else if (entry.isDirectory) {
        await traverseDirectory(entry as FileSystemDirectoryEntry, relativePath);
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
