import { useInfiniteQuery } from "@tanstack/react-query";
import { PAGE_SIZE, type Page } from "~/lib/api";

// A newest-first list fetched in PAGE_SIZE windows. The server has no total
// count, so "more" simply means the last window was full.
export function usePagedList<T>(
  queryKey: readonly unknown[],
  fetchPage: (page: Page) => Promise<T[]>,
  enabled = true,
) {
  const query = useInfiniteQuery({
    queryKey,
    queryFn: ({ pageParam }) => fetchPage({ offset: pageParam, limit: PAGE_SIZE }),
    initialPageParam: 0,
    getNextPageParam: (lastPage, pages) =>
      lastPage.length < PAGE_SIZE ? undefined : pages.length * PAGE_SIZE,
    enabled,
    retry: false,
    gcTime: 0,
  });
  return {
    items: query.data?.pages.flat(),
    isPending: query.isPending,
    isError: query.isError,
    error: query.error,
    refetch: query.refetch,
    hasMore: query.hasNextPage,
    isLoadingMore: query.isFetchingNextPage,
    loadMore: () => void query.fetchNextPage(),
  };
}
