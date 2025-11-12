import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

export interface FilterPreferences {
  selectedDevices: string[];
  minVersion: string | null;
  maxVersion: string | null;
}

const STORAGE_KEY = 'qmd-verify-filters';
const ALL_DEVICES = ['rm1', 'rm2', 'rmpp', 'rmppm'];

const DEFAULT_PREFERENCES: FilterPreferences = {
  selectedDevices: ALL_DEVICES,
  minVersion: null,
  maxVersion: null,
};

function parseDevicesFromParam(param: string | null): string[] | null {
  if (!param) return null;
  const devices = param.split(',').filter(d => ALL_DEVICES.includes(d));
  return devices.length > 0 ? devices : null;
}

function loadFromLocalStorage(): FilterPreferences | null {
  try {
    const stored = localStorage.getItem(STORAGE_KEY);
    if (!stored) return null;
    const parsed = JSON.parse(stored) as FilterPreferences;

    if (!Array.isArray(parsed.selectedDevices)) return null;

    const validDevices = parsed.selectedDevices.filter(d => ALL_DEVICES.includes(d));
    if (validDevices.length === 0) return null;

    return {
      selectedDevices: validDevices,
      minVersion: parsed.minVersion || null,
      maxVersion: parsed.maxVersion || null,
    };
  } catch {
    return null;
  }
}

function saveToLocalStorage(preferences: FilterPreferences): void {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(preferences));
  } catch (error) {
    console.error('Failed to save filter preferences:', error);
  }
}

export function useFilterPreferences() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [preferences, setPreferences] = useState<FilterPreferences>(() => {
    const devicesParam = searchParams.get('devices');
    const minVersionParam = searchParams.get('minVersion');
    const maxVersionParam = searchParams.get('maxVersion');

    const hasUrlParams = devicesParam || minVersionParam || maxVersionParam;

    if (hasUrlParams) {
      const devices = parseDevicesFromParam(devicesParam);
      return {
        selectedDevices: devices || ALL_DEVICES,
        minVersion: minVersionParam,
        maxVersion: maxVersionParam,
      };
    }

    const stored = loadFromLocalStorage();
    return stored || DEFAULT_PREFERENCES;
  });

  useEffect(() => {
    saveToLocalStorage(preferences);

    const newParams = new URLSearchParams(searchParams);

    if (preferences.selectedDevices.length === ALL_DEVICES.length) {
      newParams.delete('devices');
    } else {
      newParams.set('devices', preferences.selectedDevices.join(','));
    }

    if (preferences.minVersion) {
      newParams.set('minVersion', preferences.minVersion);
    } else {
      newParams.delete('minVersion');
    }

    if (preferences.maxVersion) {
      newParams.set('maxVersion', preferences.maxVersion);
    } else {
      newParams.delete('maxVersion');
    }

    const newParamsString = newParams.toString();
    const currentParamsString = searchParams.toString();

    if (newParamsString !== currentParamsString) {
      setSearchParams(newParams, { replace: true });
    }
  }, [preferences, setSearchParams]);

  const setSelectedDevices = (devices: string[]) => {
    const validDevices = devices.filter(d => ALL_DEVICES.includes(d));
    if (validDevices.length > 0) {
      setPreferences(prev => ({ ...prev, selectedDevices: validDevices }));
    }
  };

  const setMinVersion = (version: string | null) => {
    setPreferences(prev => ({ ...prev, minVersion: version }));
  };

  const setMaxVersion = (version: string | null) => {
    setPreferences(prev => ({ ...prev, maxVersion: version }));
  };

  const setVersionRange = (min: string | null, max: string | null) => {
    setPreferences(prev => ({ ...prev, minVersion: min, maxVersion: max }));
  };

  return {
    preferences,
    setSelectedDevices,
    setMinVersion,
    setMaxVersion,
    setVersionRange,
  };
}
