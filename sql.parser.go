package sqlparser

import (
	"regexp"
	"fmt"
	"errors"
	"reflect"
	"github.com/xwb1989/sqlparser"
	"github.com/binlaniua/kitgo"
)

var (
	pattern_chinese = regexp.MustCompile("[\u4E00-\u9FA5]+")
	pattern_as = regexp.MustCompile(`(?i)as ('|")([^'"]+)('|")`)
)

//-------------------------------------
//
//
//
//-------------------------------------
type SQLParser struct {
	sql             string
	chineseFieldMap map[string]string
	result          *SQLParserResult
}

//
//
//
//
//
func NewSQLParser(sqlString string) *SQLParser {
	//
	sp := &SQLParser{
		sql: sqlString,
		chineseFieldMap: map[string]string{},
		result: NewSQLparserResult(),
	}

	//
	sp.cleanSql()
	return sp
}

//
//
// 获取解析结果
//
//
func (sp *SQLParser) GetResult() *SQLParserResult {
	return sp.result
}

//
//
//
//
//
func (sp *SQLParser) DoParser() (*SQLParserResult, error) {
	ast, err := sqlparser.Parse(sp.sql)
	if err != nil {
		return nil, err
	}

	switch node := ast.(type){
	//
	case *sqlparser.Select:
		sp.result.sqlType = SQL_TYPE_SELECT
		sp.visitSelect(node, "")

	//
	case *sqlparser.Union:
		sp.result.sqlType = SQL_TYPE_UNION
		sp.visitUnion(node, "")

	//
	case *sqlparser.Insert:
		sp.result.sqlType = SQL_TYPE_INSERT
		sp.visitInsert(node)

	//
	case *sqlparser.Update:
		sp.result.sqlType = SQL_TYPE_UPDATE
		sp.visitUpdate(node)

	//
	case *sqlparser.Delete:
		sp.result.sqlType = SQL_TYPE_DEL
		sp.visitDelete(node)

	//
	default:
		return nil, errors.New(fmt.Sprintf("不支持类型 => %s", reflect.TypeOf(ast)))
	}
	return sp.result, nil
}



//
//
// 清理SQL
// 1. 不支持中文   not support chinese
// 2. 不支持函数   not support database function
// 3. AS 后面不能更引号 not support "as 'xxx'", support "as xxxx"
// 4. 不支持(+)连接 not support left join use (+)
//
//
func (sp *SQLParser) cleanSql() {
	count := 0
	sp.sql = pattern_chinese.ReplaceAllStringFunc(sp.sql, func(src string) string {
		alias := fmt.Sprintf("__r%d", count)
		sp.chineseFieldMap[alias] = src
		count++
		return alias
	})
	sp.sql = pattern_as.ReplaceAllStringFunc(sp.sql, func(src string) string {
		return kitgo.StringReplace(src, `"|'`, "")
	})
	sp.sql = kitgo.StringReplace(sp.sql, ";|\\(\\+\\)", "")
}

//
//
// 分析新增
//
//
func (sp *SQLParser) visitInsert(node *sqlparser.Insert) {
	sp.visitSimpleTable(node.Table, "")
	for _, column := range node.Columns {
		sp.visitExpr(column)
	}
}

//
//
// 分析更新
//
//
func (sp *SQLParser) visitUpdate(node *sqlparser.Update) {
	sp.visitSimpleTable(node.Table, "")
	for _, column := range node.Exprs {
		sp.result.AddCol(string(column.Name.Name), "", string(column.Name.Qualifier))
	}
}

//
//
// 分析删除
//
//
func (sp *SQLParser) visitDelete(node *sqlparser.Delete) {
	sp.visitSimpleTable(node.Table, "")
}

//
//
// 分析查询
//
//
func (sp *SQLParser) visitQuery(node sqlparser.SelectStatement, nodeAlias string) {
	switch s := node.(type){
	case *sqlparser.Select:
		sp.visitSelect(s, nodeAlias)
	case *sqlparser.Union:
		sp.visitUnion(s, nodeAlias)
	}
}

//
//
// 分析union
//
//
func (sp *SQLParser) visitUnion(node *sqlparser.Union, nodeAlias string) {
	sp.visitQuery(node.Left, nodeAlias)
	sp.visitQuery(node.Right, nodeAlias)
}

//
//
// 解析Select
//
//
func (sp *SQLParser) visitSelect(node *sqlparser.Select, nodeAlias string) {
	// 先解表
	for _, table := range node.From {
		sp.visitForm(table, nodeAlias)
	}

	// 再解字段
	for _, field := range node.SelectExprs {
		sp.visitExpr(field)
	}
}

//
//
// 解析表字段
//
//
func (sp *SQLParser) visitExpr(exp sqlparser.SelectExpr) {
	switch f := exp.(type){
	case *sqlparser.NonStarExpr:
		switch c := f.Expr.(type){
		//表达式
		case *sqlparser.ColName:
			sp.result.AddCol(string(c.Name), string(f.As), string(c.Qualifier))

		//带函数的表达式
		case *sqlparser.FuncExpr:
			for _, e := range c.Exprs {
				sp.visitExpr(e)
			}
		//
		case sqlparser.ValTuple:
			for _, v := range c {
				sp.visitValExp(v)
			}

		// case when
		case *sqlparser.CaseExpr:
			sp.visitValExp(c.Else)
			sp.visitValExp(c.Expr)

		//非表达式, 不管
		case sqlparser.StrVal:
		case sqlparser.NumVal:
		case *sqlparser.BinaryExpr:
		default:
			kitgo.ErrorLog.Println(reflect.TypeOf(f.Expr))
		}
	case *sqlparser.StarExpr:
		sp.result.AddCol("*", "*", string(f.TableName))
	}
}

//
//
// 解析表达式
//
//
func (sp *SQLParser)  visitValExp(exp sqlparser.ValExpr) {
	if exp == nil {
		return
	}
	switch f := exp.(type) {
	case *sqlparser.BinaryExpr:
	default:
		kitgo.ErrorLog.Println(f)
	}
}

//
//
// 分析表
//
//
func (sp *SQLParser) visitForm(table sqlparser.TableExpr, nodeAlias string) {
	switch t := table.(type) {

	//父表
	case *sqlparser.ParenTableExpr:
		sp.visitForm(t.Expr, nodeAlias)

	//左右表
	case *sqlparser.JoinTableExpr:
		sp.visitForm(t.LeftExpr, nodeAlias)
		sp.visitForm(t.RightExpr, nodeAlias)

	//真实表
	case *sqlparser.AliasedTableExpr:
		newNodeAlias := string(t.As)
		if newNodeAlias != "" {
			//如果有表明名, 那么new就是内部的表别名
			//例如:
			//	select * from (select * from XX x) t
			//      这里的话
			//		nodeAlias = t
			//		newNodeAlias = x
			//	所以要把 t 映射为 x
			sp.result.AddTableAlias(newNodeAlias, nodeAlias)
			nodeAlias = newNodeAlias
		}
		sp.visitTable(t.Expr, nodeAlias)
	default:
		kitgo.ErrorLog.Print(reflect.TypeOf(table))
	}
}

//
//
// 分析简单表
//
//
func (sp *SQLParser) visitTable(table sqlparser.SimpleTableExpr, alias string) {
	switch t := table.(type){
	//简单的表
	case *sqlparser.TableName:
		dbOwner := string(t.Qualifier)
		tableName := string(t.Name)
		sp.result.AddTable(dbOwner, tableName, alias)

	//子查询
	case *sqlparser.Subquery:
		sp.visitQuery(t.Select, alias)
	}
}

//
//
//
//
//
func (sp *SQLParser) visitSimpleTable(table *sqlparser.TableName, alias string) {
	dbOwner := string(table.Qualifier)
	tableName := string(table.Name)
	sp.result.AddTable(dbOwner, tableName, alias)
}