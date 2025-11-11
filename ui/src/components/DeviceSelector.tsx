import { Label } from '@/components/ui/label';
import { Switch } from '@/components/ui/switch';

const DEVICE_INFO: Record<string, { short: string; full: string }> = {
  'rm1': { short: 'rM1', full: 'reMarkable 1' },
  'rm2': { short: 'rM2', full: 'reMarkable 2' },
  'rmpp': { short: 'rMPP', full: 'Paper Pro' },
  'rmppm': { short: 'rMPPM', full: 'Paper Pro Move' },
};

interface DeviceSelectorProps {
  selectedDevices: string[];
  onChange: (devices: string[]) => void;
}

const FIXED_DEVICE_ORDER = ['rm1', 'rm2', 'rmpp', 'rmppm'];

export function DeviceSelector({ selectedDevices, onChange }: DeviceSelectorProps) {
  const handleToggle = (device: string, checked: boolean) => {
    if (checked) {
      onChange([...selectedDevices, device]);
    } else {
      const filtered = selectedDevices.filter(d => d !== device);
      if (filtered.length > 0) {
        onChange(filtered);
      }
    }
  };

  return (
    <div>
      <Label className="text-sm font-medium">Devices</Label>
      <div className="grid grid-cols-2 @2xl:grid-cols-4 gap-4 gap-x-8 mt-4">
        {FIXED_DEVICE_ORDER.map(device => {
          const info = DEVICE_INFO[device];
          const isChecked = selectedDevices.includes(device);

          return (
            <div key={device} className="flex items-center space-x-2">
              <Switch
                id={`device-${device}`}
                checked={isChecked}
                onCheckedChange={(checked) => handleToggle(device, checked)}
              />
              <Label
                htmlFor={`device-${device}`}
                className="text-sm font-normal cursor-pointer"
              >
                {info ? info.full : device}
              </Label>
            </div>
          );
        })}
      </div>
    </div>
  );
}
