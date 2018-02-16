package controller

import (
	"net/http"

	"github.com/fabric8-services/fabric8-wit/app"
	"github.com/fabric8-services/fabric8-wit/application"
	"github.com/fabric8-services/fabric8-wit/errors"
	"github.com/fabric8-services/fabric8-wit/jsonapi"
	"github.com/fabric8-services/fabric8-wit/rest"
	"github.com/fabric8-services/fabric8-wit/space"
	"github.com/fabric8-services/fabric8-wit/workitem"
	"github.com/goadesign/goa"
)

// WorkItemTypeGroupController implements the work_item_type_group resource.
type WorkItemTypeGroupController struct {
	*goa.Controller
	db application.DB
}

// APIWorkItemTypeGroups is the type constant used when referring to work item
// type group relationships in JSONAPI
var APIWorkItemTypeGroups = "workitemtypegroups"

// NewWorkItemTypeGroupController creates a work_item_type_group controller.
func NewWorkItemTypeGroupController(service *goa.Service, db application.DB) *WorkItemTypeGroupController {
	return &WorkItemTypeGroupController{
		Controller: service.NewController("WorkItemTypeGroupController"),
		db:         db,
	}
}

// Show runs the list action.
func (c *WorkItemTypeGroupController) Show(ctx *app.ShowWorkItemTypeGroupContext) error {
	// TODO(kwk): Replace with loading from DB once type groups are persistently
	// stored in there.
	for _, group := range workitem.TypeGroups() {
		if group.ID == ctx.GroupID {
			return ctx.OK(&app.WorkItemTypeGroupSingle{
				Data: ConvertTypeGroup(ctx.Request, group),
			})
		}
	}
	return jsonapi.JSONErrorResponse(ctx, errors.NewNotFoundError("type group", ctx.GroupID.String()))
}

// ConvertTypeGroup converts WorkitemTypeGroup model to a response resource
// object for jsonapi.org specification
func ConvertTypeGroup(request *http.Request, tg workitem.WorkItemTypeGroup) *app.WorkItemTypeGroupData {

	spaceTemplateID := space.SystemSpace
	spaceTemplateIDStr := spaceTemplateID.String()
	workitemtypes := "workitemtypes"
	workItemTypeGroupRelatedURL := rest.AbsoluteURL(request, app.WorkItemTypeGroupHref(tg.ID))
	createdAt := tg.CreatedAt.UTC()
	updatedAt := tg.UpdatedAt.UTC()
	// Every work item type group except the one in the "iteration" bucket are
	// meant to be shown in the sidebar.
	showInSidebar := (tg.Bucket != workitem.BucketIteration)

	res := &app.WorkItemTypeGroupData{
		ID:   &tg.ID,
		Type: APIWorkItemTypeGroups,
		Links: &app.GenericLinks{
			Related: &workItemTypeGroupRelatedURL,
		},
		Attributes: &app.WorkItemTypeGroupAttributes{
			Bucket:        tg.Bucket.String(),
			Name:          tg.Name,
			Icon:          tg.Icon,
			CreatedAt:     &createdAt,
			UpdatedAt:     &updatedAt,
			ShowInSidebar: &showInSidebar,
		},
		Relationships: &app.WorkItemTypeGroupRelationships{
			TypeList: &app.RelationGenericList{
				Data: make([]*app.GenericData, len(tg.TypeList)),
			},
			SpaceTemplate: &app.RelationGeneric{
				Data: &app.GenericData{
					ID:   &spaceTemplateIDStr,
					Type: &APISpaceTemplates,
				},
			},
		},
	}

	for i, witID := range tg.TypeList {
		// witRelatedURL := rest.AbsoluteURL(request, app.WorkitemtypeHref(space.SystemSpace, witID))
		idStr := witID.String()
		res.Relationships.TypeList.Data[i] = &app.GenericData{
			ID:   &idStr,
			Type: &workitemtypes,
			// Links: &app.GenericLinks{
			// Related: &witRelatedURL,
			// },
		}
	}
	return res
}
