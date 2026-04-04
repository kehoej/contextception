"""Search API endpoints."""
from webapp.catalog.search import SearchEngine
from webapp.api.serializers import BaseSerializer

def search(request):
    """Search across products and content."""
    query = request.args.get('q', '')
    engine = SearchEngine()
    results = engine.search(query)
    serializer = BaseSerializer(results)
    return {'results': serializer.serialize()}

def autocomplete(request):
    """Autocomplete search suggestions."""
    query = request.args.get('q', '')
    engine = SearchEngine()
    suggestions = engine.suggest(query)
    return {'suggestions': suggestions}
