package location

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const collectionName = "locations"

type mapDoc struct {
	X     float64 `bson:"x"`
	Z     float64 `bson:"z"`
	Scale float64 `bson:"scale"`
}

type assetsDoc struct {
	Icon string `bson:"icon"`
}

type mongoDoc struct {
	ID                primitive.ObjectID `bson:"_id,omitempty"`
	WorldID           string             `bson:"worldId"`
	Slug              string             `bson:"slug"`
	Name              string             `bson:"name"`
	CountryCode       string             `bson:"countryCode,omitempty"`
	Region            string             `bson:"region,omitempty"`
	Kind              string             `bson:"kind"`
	BoardIndex        int                `bson:"boardIndex"`
	Price             int                `bson:"price"`
	Rents             []int              `bson:"rents,omitempty"`
	HouseCost         *int               `bson:"houseCost,omitempty"`
	ColorGroup        *string            `bson:"colorGroup,omitempty"`
	SpecialType       *string            `bson:"specialType,omitempty"`
	UtilityMultiplier []int              `bson:"utilityMultiplier,omitempty"`
	TaxAmount         *int               `bson:"taxAmount,omitempty"`
	PassBonus         *int               `bson:"passBonus,omitempty"`
	Map               mapDoc             `bson:"map"`
	Assets            assetsDoc          `bson:"assets"`
	HubID             string             `bson:"hubId"`
	EnterRadius       float64            `bson:"enterRadius"`
	Description       string             `bson:"description,omitempty"`
	Svgcities         string             `bson:"svgcities,omitempty"`
	SvgcitiesPath     string             `bson:"svgcitiesPath,omitempty"`
	SvgcitiesURL      string             `bson:"svgcitiesUrl,omitempty"`
	Attribution       string             `bson:"attribution,omitempty"`
	Symbol            string             `bson:"symbol,omitempty"`
	SymbolStory       string             `bson:"symbolStory,omitempty"`
	About             string             `bson:"about,omitempty"`
	AboutShort        string             `bson:"aboutShort,omitempty"`
	Tags              []string           `bson:"tags,omitempty"`
	LandmarkCategory  string             `bson:"landmarkCategory,omitempty"`
}

// MongoRepository implements Repository with MongoDB.
type MongoRepository struct {
	col *mongo.Collection
}

// NewMongoRepository binds to the locations collection.
func NewMongoRepository(db *mongo.Database) *MongoRepository {
	return &MongoRepository{col: db.Collection(collectionName)}
}

func (r *MongoRepository) EnsureIndexes(ctx context.Context) error {
	models := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "worldId", Value: 1}, {Key: "boardIndex", Value: 1}},
		},
		{
			Keys:    bson.D{{Key: "worldId", Value: 1}, {Key: "slug", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}
	_, err := r.col.Indexes().CreateMany(ctx, models)
	if err != nil {
		return fmt.Errorf("location indexes: %w", err)
	}
	return nil
}

func (r *MongoRepository) ListByWorldID(ctx context.Context, worldID string) ([]Location, error) {
	opts := options.Find().SetSort(bson.D{{Key: "boardIndex", Value: 1}})
	cur, err := r.col.Find(ctx, bson.M{"worldId": worldID}, opts)
	if err != nil {
		return nil, fmt.Errorf("location list: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []Location
	for cur.Next(ctx) {
		var doc mongoDoc
		if err := cur.Decode(&doc); err != nil {
			return nil, fmt.Errorf("location decode: %w", err)
		}
		out = append(out, *fromDoc(doc))
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("location cursor: %w", err)
	}
	return out, nil
}

func (r *MongoRepository) FindByID(ctx context.Context, id string) (*Location, error) {
	oid, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, ErrNotFound
	}
	var doc mongoDoc
	err = r.col.FindOne(ctx, bson.M{"_id": oid}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("location find id: %w", err)
	}
	return fromDoc(doc), nil
}

func (r *MongoRepository) FindByWorldAndSlug(ctx context.Context, worldID, slug string) (*Location, error) {
	var doc mongoDoc
	err := r.col.FindOne(ctx, bson.M{"worldId": worldID, "slug": slug}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("location find slug: %w", err)
	}
	return fromDoc(doc), nil
}

func (r *MongoRepository) ListWorlds(ctx context.Context) ([]WorldSummary, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$worldId"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("location worlds: %w", err)
	}
	defer func() { _ = cur.Close(ctx) }()

	var out []WorldSummary
	for cur.Next(ctx) {
		var row struct {
			ID    string `bson:"_id"`
			Count int    `bson:"count"`
		}
		if err := cur.Decode(&row); err != nil {
			return nil, fmt.Errorf("location worlds decode: %w", err)
		}
		out = append(out, WorldSummary{WorldID: row.ID, Count: row.Count})
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("location worlds cursor: %w", err)
	}
	return out, nil
}

func (r *MongoRepository) ReplaceAll(ctx context.Context, docs []Location) (int, error) {
	if _, err := r.col.DeleteMany(ctx, bson.M{}); err != nil {
		return 0, fmt.Errorf("location clear: %w", err)
	}
	if len(docs) == 0 {
		return 0, nil
	}
	models := make([]any, 0, len(docs))
	for i := range docs {
		models = append(models, toDoc(docs[i]))
	}
	res, err := r.col.InsertMany(ctx, models)
	if err != nil {
		return 0, fmt.Errorf("location insert: %w", err)
	}
	return len(res.InsertedIDs), nil
}

func fromDoc(doc mongoDoc) *Location {
	return &Location{
		ID:                doc.ID.Hex(),
		WorldID:           doc.WorldID,
		Slug:              doc.Slug,
		Name:              doc.Name,
		CountryCode:       doc.CountryCode,
		Region:            doc.Region,
		Kind:              doc.Kind,
		BoardIndex:        doc.BoardIndex,
		Price:             doc.Price,
		Rents:             doc.Rents,
		HouseCost:         doc.HouseCost,
		ColorGroup:        doc.ColorGroup,
		SpecialType:       doc.SpecialType,
		UtilityMultiplier: doc.UtilityMultiplier,
		TaxAmount:         doc.TaxAmount,
		PassBonus:         doc.PassBonus,
		Map: MapPose{
			X:     doc.Map.X,
			Z:     doc.Map.Z,
			Scale: doc.Map.Scale,
		},
		Assets:           Assets{Icon: doc.Assets.Icon},
		HubID:            doc.HubID,
		EnterRadius:      doc.EnterRadius,
		Description:      doc.Description,
		Svgcities:        doc.Svgcities,
		SvgcitiesPath:    doc.SvgcitiesPath,
		SvgcitiesURL:     doc.SvgcitiesURL,
		Attribution:      doc.Attribution,
		Symbol:           doc.Symbol,
		SymbolStory:      doc.SymbolStory,
		About:            doc.About,
		AboutShort:       doc.AboutShort,
		Tags:             doc.Tags,
		LandmarkCategory: doc.LandmarkCategory,
	}
}

func toDoc(loc Location) mongoDoc {
	doc := mongoDoc{
		WorldID:           loc.WorldID,
		Slug:              loc.Slug,
		Name:              loc.Name,
		CountryCode:       loc.CountryCode,
		Region:            loc.Region,
		Kind:              loc.Kind,
		BoardIndex:        loc.BoardIndex,
		Price:             loc.Price,
		Rents:             loc.Rents,
		HouseCost:         loc.HouseCost,
		ColorGroup:        loc.ColorGroup,
		SpecialType:       loc.SpecialType,
		UtilityMultiplier: loc.UtilityMultiplier,
		TaxAmount:         loc.TaxAmount,
		PassBonus:         loc.PassBonus,
		Map: mapDoc{
			X:     loc.Map.X,
			Z:     loc.Map.Z,
			Scale: loc.Map.Scale,
		},
		Assets:           assetsDoc{Icon: loc.Assets.Icon},
		HubID:            loc.HubID,
		EnterRadius:      loc.EnterRadius,
		Description:      loc.Description,
		Svgcities:        loc.Svgcities,
		SvgcitiesPath:    loc.SvgcitiesPath,
		SvgcitiesURL:     loc.SvgcitiesURL,
		Attribution:      loc.Attribution,
		Symbol:           loc.Symbol,
		SymbolStory:      loc.SymbolStory,
		About:            loc.About,
		AboutShort:       loc.AboutShort,
		Tags:             loc.Tags,
		LandmarkCategory: loc.LandmarkCategory,
	}
	if loc.ID != "" {
		if oid, err := primitive.ObjectIDFromHex(loc.ID); err == nil {
			doc.ID = oid
		}
	}
	return doc
}
