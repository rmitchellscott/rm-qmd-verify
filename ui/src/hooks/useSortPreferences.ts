import { useEffect, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

export type SortMethod = 'dependency' | 'failures' | 'alphabetical';

const VALID_SORT_METHODS: SortMethod[] = ['dependency', 'failures', 'alphabetical'];

export function useSortPreferences(defaultSort: SortMethod) {
  const [searchParams, setSearchParams] = useSearchParams();
  const [sortMethod, setSortMethod] = useState<SortMethod>(() => {
    const sortParam = searchParams.get('sort');

    if (sortParam && VALID_SORT_METHODS.includes(sortParam as SortMethod)) {
      return sortParam as SortMethod;
    }

    return defaultSort;
  });

  useEffect(() => {
    const newParams = new URLSearchParams(searchParams);
    newParams.set('sort', sortMethod);

    const newParamsString = newParams.toString();
    const currentParamsString = searchParams.toString();

    if (newParamsString !== currentParamsString) {
      setSearchParams(newParams, { replace: true });
    }
  }, [sortMethod, searchParams, setSearchParams]);

  return {
    sortMethod,
    setSortMethod,
  };
}
